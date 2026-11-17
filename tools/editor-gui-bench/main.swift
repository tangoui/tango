// Build: swiftc -swift-version 5 -parse-as-library CaptureCore.swift main.swift -o gui-probe
// Local-only capture/input probe. Captures a display crop of the owned target,
// never stores the full desktop. Frame times are WindowServer displayTime.
import AppKit
import ScreenCaptureKit
import CoreMedia
import CoreVideo
import Carbon
import Darwin

func nowNS() -> UInt64 {
    var t = mach_timebase_info_data_t(); mach_timebase_info(&t)
    return hostNanoseconds(mach_absolute_time(), numer: UInt64(t.numer), denom: UInt64(t.denom))!
}
func ticksNS(_ ticks: UInt64) -> UInt64 {
    var t = mach_timebase_info_data_t(); mach_timebase_info(&t)
    return hostNanoseconds(ticks, numer: UInt64(t.numer), denom: UInt64(t.denom))!
}
struct ProbeError: Error, CustomStringConvertible { let description: String; init(_ s: String) { description = s } }

func currentOcclusion(target: UInt32, region: CGRect) -> Occlusion {
    guard let raw=CGWindowListCopyWindowInfo([.optionOnScreenOnly,.excludeDesktopElements], kCGNullWindowID) as? [[String:Any]] else { return .missingTarget }
    let ws=raw.compactMap { w -> WindowSnapshot? in
        guard let id=w[kCGWindowNumber as String] as? NSNumber,
              let b=w[kCGWindowBounds as String] as? [String:Any],
              let r=CGRect(dictionaryRepresentation:b as CFDictionary) else { return nil }
        return WindowSnapshot(id:id.uint32Value, rect:r, alpha:(w[kCGWindowAlpha as String] as? NSNumber)?.doubleValue ?? 0)
    }
    return occlusion(ws, target:target, region:region)
}
final class JSONLog {
    let file: FileHandle; let lock = NSLock()
    init(_ url: URL) throws {
        guard FileManager.default.createFile(atPath: url.path, contents: nil) else { throw ProbeError("cannot create log") }
        file = try FileHandle(forWritingTo: url)
    }
    func emit(_ row: [String: Any]) {
        lock.lock(); defer { lock.unlock() }
        if let d = try? JSONSerialization.data(withJSONObject: row, options: [.sortedKeys]) {
            file.write(d); file.write(Data([10]))
        }
    }
    deinit { try? file.close() }
}
struct Action: Codable {
    var id: Int; var kind: String; var keycode: UInt16?; var dy: Int32?; var flags: UInt64?
    var x: Double?; var y: Double?; var waitMS: Int?; var patches: [Patch]?
    // Record mode: per region, the per-pixel maximum of the display from
    // settleMS after posting to the end of the wait (a dark caret blinking
    // on a light theme drops out), written as rec-<id>-<i>.bgra.
    var record: [Patch]?; var settleMS: Int?
}
struct Job: Codable {
    var pid: Int32; var windowID: UInt32; var displayID: UInt32?
    var crop: [Double]?; var durationMS: Int?; var snapshot: Bool?; var calibration: Bool?; var isolatedWindow: Bool?
    var actions: [Action]?; var inputSource: String?
    // Alignment: after action alignAfter (which records the regions), find
    // the whole-body shift from the record pass's baseline crops alignTo
    // (one file per region) and move every later patch by it. A shift that
    // does not bring the body within mean distance 1 fails the job.
    var alignTo: [String]?; var alignAfter: Int?
}
func inputSourceID(_ src: TISInputSource) -> String {
    guard let p = TISGetInputSourceProperty(src, kTISPropertyInputSourceID) else { return "unknown" }
    return Unmanaged<CFString>.fromOpaque(p).takeUnretainedValue() as String
}
func selectSource(_ name: String) throws {
    let all = TISCreateInputSourceList(nil, false).takeRetainedValue() as NSArray
    for obj in all {
        let src = obj as! TISInputSource
        if inputSourceID(src) == name {
            guard TISSelectInputSource(src) == noErr else { throw ProbeError("cannot select input source \(name)") }
            return
        }
    }
    throw ProbeError("input source not enabled: \(name)")
}

final class Capture: NSObject, SCStreamOutput, SCStreamDelegate {
    let log: JSONLog; let out: URL; let job: Job; let scale: Double; let region: CGRect
    let lock = NSLock()
    var action: Action?; var templates: [[UInt8]] = []
    var gotFrame = false; var done = false; var streamError: String?
    var lastTime: UInt64 = 0
    var recFrom: UInt64 = .max; var recStarted = false
    var recAcc: [[UInt8]] = []; var lastCrops: [[UInt8]] = []; var recFrames = 0
    // The job's record regions, cropped from every complete frame from the
    // start: a display that stays idle until a record interval is then
    // still represented by its last complete frame.
    let recordRegions: [Patch]?
    init(log: JSONLog, out: URL, job: Job, scale: Double, region: CGRect) { self.log = log; self.out = out; self.job = job; self.scale = scale; self.region = region; recordRegions = job.actions?.first(where: { $0.record != nil })?.record }
    func arm(_ a: Action) throws {
        let ts = try (a.patches ?? []).map { p -> [UInt8] in
            guard let file = p.template else { throw ProbeError("body patch needs a template") }
            return [UInt8](try Data(contentsOf: URL(fileURLWithPath: file)))
        }
        lock.lock(); defer { lock.unlock() }
        action = a; templates = ts; done = false
        recFrom = .max; recStarted = false; recAcc = []; recFrames = 0
    }
    func startRecord(at t: UInt64) { lock.lock(); recFrom = t; lock.unlock() }
    // takeRecord returns the regions' maxima and how many complete frames
    // went into them; with no complete frame after settling the display was
    // unchanged (idle) and the last complete frame before stands for it.
    func takeRecord() -> ([[UInt8]], Int) {
        lock.lock(); defer { lock.unlock() }
        let r = recStarted ? (recAcc, recFrames) : (lastCrops, lastCrops.isEmpty ? 0 : 1)
        recFrom = .max; recStarted = false; recAcc = []; recFrames = 0
        return r
    }
    func completed() -> Bool { lock.lock(); defer { lock.unlock() }; return done }
    func failure() -> String? { lock.lock(); defer { lock.unlock() }; return streamError }
    func hasFrame() -> Bool { lock.lock(); defer { lock.unlock() }; return gotFrame }
    func didStop(_ message: String) { lock.lock(); defer { lock.unlock() }; streamError = message }
    func stream(_ stream: SCStream, didStopWithError error: Error) { didStop(error.localizedDescription); log.emit(["type":"stream-error", "error":error.localizedDescription]) }
    func stream(_ stream: SCStream, didOutputSampleBuffer sample: CMSampleBuffer, of type: SCStreamOutputType) {
        guard type == .screen, sample.isValid,
              let infos = CMSampleBufferGetSampleAttachmentsArray(sample, createIfNecessary: false) as? [[SCStreamFrameInfo: Any]], let info = infos.first else { return }
        // A newly raised system/foreign window must never be written into
        // evidence, even if it appeared after startCapture. No pixel handling
        // or snapshot write until current WindowServer z-order is clear.
        if captureRequiresUnobscuredDisplay(isolatedWindow:job.isolatedWindow == true || job.snapshot == true) {
            guard currentOcclusion(target:job.windowID,region:region) == .clear else {
                log.emit(["type":"privacy-blocked", "callback":nowNS(), "reason":"target obscured or missing"])
                didStop("target obscured or missing")
                return
            }
        }
        let rawStatus = (info[.status] as? NSNumber)?.intValue ?? -1
        let status = [0:"complete", 1:"idle", 2:"blank", 3:"suspended", 4:"started", 5:"stopped"][rawStatus] ?? "unknown"
        let rawTime = (info[.displayTime] as? NSNumber)?.uint64Value
        let time = rawTime.map(ticksNS)
        let callback = nowNS()
        guard let image = sample.imageBuffer else {
            log.emit(["type":"frame", "status":status, "time":time as Any? ?? NSNull(), "callback":callback, "invalid":"no-image"]); return
        }
        let w = CVPixelBufferGetWidth(image), h = CVPixelBufferGetHeight(image)
        lock.lock(); defer { lock.unlock() }
        var row: [String:Any] = ["type":"frame", "status":status, "time":time as Any? ?? NSNull(), "callback":callback,
            "width":w, "height":h, "scale":(info[.scaleFactor] as? NSNumber)?.doubleValue ?? scale, "id":action?.id as Any? ?? NSNull()]
        if let time = time { if time < lastTime { row["invalid"] = "clock-backwards" }; lastTime = time }
        guard status == "complete" else { log.emit(row); return }
        CVPixelBufferLockBaseAddress(image, .readOnly); defer { CVPixelBufferUnlockBaseAddress(image, .readOnly) }
        guard let base = CVPixelBufferGetBaseAddress(image) else { row["invalid"] = "no-pixels"; log.emit(row); return }
        let stride = CVPixelBufferGetBytesPerRow(image)
        if job.calibration == true {
            let px=base.advanced(by:(h/2)*stride+(w/2)*4).assumingMemoryBound(to:UInt8.self)
            row["calibrationCenterBGRA"] = Array(UnsafeBufferPointer(start:px,count:4))
        }
        if job.snapshot == true && !gotFrame {
            var bytes = Data(capacity: w * h * 4)
            for y in 0..<h { bytes.append(base.advanced(by:y*stride).assumingMemoryBound(to:UInt8.self), count:w*4) }
            do { try bytes.write(to:out.appendingPathComponent("snapshot.bgra")) }
            catch { row["invalid"] = "snapshot-write: \(error)" }
            row["snapshot"] = "snapshot.bgra"
        }
        gotFrame = true
        if let regs = action?.record ?? recordRegions, !regs.isEmpty {
            var crops: [[UInt8]] = []
            for p in regs {
                guard p.x >= 0, p.y >= 0, p.width > 0, p.height > 0, p.x+p.width<=w, p.y+p.height<=h else { row["invalid"]="record-bounds"; crops=[]; break }
                var pix: [UInt8] = []; pix.reserveCapacity(p.width*p.height*4)
                for y in p.y..<(p.y+p.height) {
                    pix.append(contentsOf:UnsafeBufferPointer(start:base.advanced(by:y*stride+p.x*4).assumingMemoryBound(to:UInt8.self), count:p.width*4))
                }
                crops.append(pix)
            }
            if !crops.isEmpty, let t = time {
                if t < recFrom { lastCrops = crops }
                else {
                    if !recStarted { recAcc = lastCrops.count == crops.count ? lastCrops : crops; recStarted = true; recFrames = lastCrops.count == crops.count ? 1 : 0 }
                    for i in crops.indices { for j in crops[i].indices where crops[i][j] > recAcc[i][j] { recAcc[i][j] = crops[i][j] } }
                    recFrames += 1; row["recorded"] = true
                    lastCrops = crops
                }
            }
        }
        var scores: [Double] = []
        if let a = action {
            for (i, p) in (a.patches ?? []).enumerated() {
                guard p.x >= 0, p.y >= 0, p.width > 0, p.height > 0, p.x+p.width<=w, p.y+p.height<=h else { row["invalid"]="patch-bounds"; continue }
                var pix: [UInt8] = []; pix.reserveCapacity(p.width*p.height*4)
                for y in p.y..<(p.y+p.height) {
                    let start = base.advanced(by:y*stride+p.x*4).assumingMemoryBound(to:UInt8.self)
                    pix.append(contentsOf:UnsafeBufferPointer(start:start, count:p.width*4))
                }
                scores.append(patchDistance(pix, templates[i]) ?? Double.infinity)
            }
        }
        let matches = scores.filter { $0.isFinite && $0 <= 3 }.count
        row["matches"] = matches
        row["scores"] = scores.map { $0.isFinite ? $0 : -1 }
        if let a = action, !(a.patches ?? []).isEmpty, matches == a.patches!.count { done = true }
        log.emit(row)
    }
}

@main
@MainActor
struct Main {
    static func main() async {
        do { try await run() }
        catch { fputs("gui-probe: \(error)\n", stderr); exit(1) }
    }
    static func run() async throws {
        _ = NSApplication.shared
        let args = CommandLine.arguments
        func option(_ name: String) -> String? { guard let i=args.firstIndex(of:name), i+1<args.count else { return nil }; return args[i+1] }
        if args.contains("--permissions") {
            print("screen-recording=\(CGPreflightScreenCaptureAccess()) accessibility=\(AXIsProcessTrusted())")
            print("input-source=\(inputSourceID(TISCopyCurrentKeyboardInputSource().takeRetainedValue()))")
            return
        }
        if let id=option("--select-input") {
            try selectSource(id)
            print("input-source=\(inputSourceID(TISCopyCurrentKeyboardInputSource().takeRetainedValue()))")
            return
        }
        guard CGPreflightScreenCaptureAccess() else { throw ProbeError("screen recording permission denied (not requested automatically)") }
        let content = try await SCShareableContent.excludingDesktopWindows(true, onScreenWindowsOnly:true)
        if args.contains("--list") {
            let rows = content.windows.filter { $0.owningApplication != nil }.map { w -> [String:Any] in
                ["pid":w.owningApplication!.processID, "windowID":w.windowID, "app":w.owningApplication!.applicationName,
                 "title":w.title ?? "", "layer":w.windowLayer, "frame":[w.frame.minX,w.frame.minY,w.frame.width,w.frame.height]]
            }
            print(String(data:try JSONSerialization.data(withJSONObject:rows, options:[.sortedKeys]), encoding:.utf8)!)
            return
        }
        guard let jobFile=option("--job"), let outDir=option("--out") else { throw ProbeError("use --permissions, --list, or --job job.json --out directory") }
        let job = try JSONDecoder().decode(Job.self, from:Data(contentsOf:URL(fileURLWithPath:jobFile)))
        guard let window=content.windows.first(where:{$0.windowID==job.windowID && $0.owningApplication?.processID==job.pid}) else { throw ProbeError("owned target window not on screen") }
        guard let display=content.displays.first(where:{ job.displayID != nil ? $0.displayID==job.displayID! : $0.frame.intersects(window.frame) }) else { throw ProbeError("target window is not on a display") }
        let region: CGRect
        if let c=job.crop, c.count==4 { region=CGRect(x:c[0],y:c[1],width:c[2],height:c[3]) }
        else { region=window.frame.intersection(display.frame) }
        guard window.frame.contains(region), region.width>0, region.height>0 else { throw ProbeError("capture crop must stay within owned target window") }
        if !(job.isolatedWindow == true || job.snapshot == true) && (job.actions ?? []).isEmpty {
            guard currentOcclusion(target:window.windowID,region:region) == .clear else {
                throw ProbeError("capture refused: target missing or overlapped by a front window")
            }
        }
        let out = URL(fileURLWithPath:outDir); try FileManager.default.createDirectory(at:out, withIntermediateDirectories:true)
        let log = try JSONLog(out.appendingPathComponent("capture.ndjson"))
        // Disk snapshots use an isolated-window filter as an additional
        // privacy barrier against an overlay racing the z-order check. They
        // are visual evidence only, never the latency capture source.
        let isolated = job.isolatedWindow == true || job.snapshot == true
        let filter = isolated ? SCContentFilter(desktopIndependentWindow:window) : SCContentFilter(display:display, excludingWindows:[])
        let scale: Double
        if #available(macOS 14.0, *) { scale=Double(filter.pointPixelScale) }
        else { throw ProbeError("probe requires macOS14+ for authoritative capture scale") }
        guard scale > 0 else { throw ProbeError("invalid display pointPixelScale") }
        let conf = SCStreamConfiguration()
        conf.sourceRect = isolated ? region.offsetBy(dx:-window.frame.minX,dy:-window.frame.minY) : region.offsetBy(dx:-display.frame.minX,dy:-display.frame.minY)
        conf.width=Int((region.width*scale).rounded()); conf.height=Int((region.height*scale).rounded())
        conf.pixelFormat=kCVPixelFormatType_32BGRA; conf.showsCursor=false; conf.capturesAudio=false
        conf.minimumFrameInterval=CMTime(value:1,timescale:60); conf.queueDepth=5
        let sink=Capture(log:log,out:out,job:job,scale:scale,region:region)
        let stream=SCStream(filter:filter, configuration:conf, delegate:sink)
        try stream.addStreamOutput(sink, type:.screen, sampleHandlerQueue:DispatchQueue(label:"gui-probe.frames"))
        guard let app=NSRunningApplication(processIdentifier:job.pid) else { throw ProbeError("target process exited") }
        let originalApp = NSWorkspace.shared.frontmostApplication
        let originalSource=TISCopyCurrentKeyboardInputSource().takeRetainedValue()
        defer { TISSelectInputSource(originalSource); if let a=originalApp, !a.isTerminated { a.activate(options:[]) } }
        if !(job.actions ?? []).isEmpty {
            guard AXIsProcessTrusted() else { throw ProbeError("accessibility permission denied") }
            app.activate(options:[])
            let focusDeadline=nowNS()+2_000_000_000
            while NSWorkspace.shared.frontmostApplication?.processIdentifier != job.pid && nowNS()<focusDeadline {
                try await Task.sleep(nanoseconds:20_000_000)
            }
            guard NSWorkspace.shared.frontmostApplication?.processIdentifier == job.pid else { throw ProbeError("target activation did not obtain focus") }
            if captureRequiresUnobscuredDisplay(isolatedWindow:isolated) {
                let oc=currentOcclusion(target:window.windowID,region:region)
                guard oc == .clear else { throw ProbeError("capture refused: target \(oc == .missingTarget ? "missing from CGWindowList" : "overlapped by a front window")") }
            }
            if let src=job.inputSource {
                try selectSource(src)
                // The target's text input context rebinds to the new source
                // asynchronously; keys sent at once still reach the old one.
                try await Task.sleep(nanoseconds:400_000_000)
            }
        }
        log.emit(["type":"metadata", "pid":job.pid, "windowID":window.windowID, "displayID":display.displayID,
            "crop":[region.minX,region.minY,region.width,region.height], "width":conf.width, "height":conf.height,
            "scale":scale,"host":nowNS(),"source":isolated ? "isolated-window-functional-not-display-latency" : "display-crop",
            "inputSource":inputSourceID(TISCopyCurrentKeyboardInputSource().takeRetainedValue())])
        try await stream.startCapture()
        do {
            let readyDeadline=nowNS()+3_000_000_000
            while !sink.hasFrame() && nowNS()<readyDeadline {
                if let e=sink.failure() { throw ProbeError(e) }
                try await Task.sleep(nanoseconds:20_000_000)
            }
            guard sink.hasFrame() else { throw ProbeError("no complete capture frame") }
            var alignDX = 0, alignDY = 0
            for a0 in job.actions ?? [] {
                var a = a0
                if let ps = a.patches { a.patches = ps.map { shifted($0, dx: alignDX, dy: alignDY) } }
                guard NSWorkspace.shared.frontmostApplication?.processIdentifier==job.pid else { let f=NSWorkspace.shared.frontmostApplication; throw ProbeError("focus left target (frontmost: \(f?.bundleIdentifier ?? f?.localizedName ?? "none"))") }
                try sink.arm(a)
                let b=nowNS()
                try post(a)
                let e=nowNS(), deadline=e+UInt64(a.waitMS ?? 1000)*1_000_000
                if a.record != nil { sink.startRecord(at:e+UInt64(a.settleMS ?? 0)*1_000_000) }
                log.emit(["type":"input", "id":a.id,"kind":a.kind,"postedBefore":b,"postedAfter":e,"deadline":deadline,
                    "regions":(a.patches ?? []).count,"width":conf.width,"height":conf.height,"scale":scale])
                while nowNS()<deadline && !sink.completed() {
                    if let err=sink.failure() { throw ProbeError(err) }
                    try await Task.sleep(nanoseconds:5_000_000)
                }
                log.emit(["type":"input-end","id":a.id,"host":nowNS(),"matched":sink.completed()])
                if let regs=a.record {
                    let (acc,n)=sink.takeRecord()
                    for (i,p) in regs.enumerated() where i < acc.count {
                        let name="rec-\(a.id)-\(i).bgra"
                        try Data(acc[i]).write(to:out.appendingPathComponent(name))
                        log.emit(["type":"record","id":a.id,"region":i,"x":p.x,"y":p.y,"width":p.width,"height":p.height,"frames":n,"file":name])
                    }
                    if acc.isEmpty { log.emit(["type":"record","id":a.id,"frames":0,"invalid":"no-frame"]) }
                    if a.id == job.alignAfter, let files = job.alignTo {
                        let refs = try files.map { [UInt8](try Data(contentsOf: URL(fileURLWithPath: $0))) }
                        guard let s = bestShift(refs: refs, curs: acc, widths: regs.map { $0.width }, heights: regs.map { $0.height }, maxShift: 4) else {
                            throw ProbeError("alignment: reference and current crops differ in size")
                        }
                        log.emit(["type":"align","id":a.id,"dx":s.dx,"dy":s.dy,"distance":s.distance])
                        guard s.distance <= 1 else { throw ProbeError("alignment failed: best shift (\(s.dx),\(s.dy)) leaves distance \(s.distance)") }
                        alignDX = s.dx; alignDY = s.dy
                    }
                }
            }
            if (job.actions ?? []).isEmpty { try await Task.sleep(nanoseconds:UInt64(job.durationMS ?? 300)*1_000_000) }
            try await stream.stopCapture()
        } catch { try? await stream.stopCapture(); throw error }
        log.emit(["type":"exit","code":0,"host":nowNS()])
    }
    static func post(_ a: Action) throws {
        let source=CGEventSource(stateID:.hidSystemState)
        if a.kind=="key", let key=a.keycode {
            guard let down=CGEvent(keyboardEventSource:source,virtualKey:key,keyDown:true), let up=CGEvent(keyboardEventSource:source,virtualKey:key,keyDown:false) else { throw ProbeError("cannot allocate key event") }
            down.flags=CGEventFlags(rawValue:a.flags ?? 0); up.flags=down.flags
            down.post(tap:.cghidEventTap); up.post(tap:.cghidEventTap)
        } else if a.kind=="scroll" {
            guard let ev=CGEvent(scrollWheelEvent2Source:source,units:.pixel,wheelCount:1,wheel1:a.dy ?? -40,wheel2:0,wheel3:0) else { throw ProbeError("cannot allocate scroll event") }
            ev.location=CGPoint(x:a.x ?? 200,y:a.y ?? 200); ev.post(tap:.cghidEventTap)
        } else if a.kind=="click" {
            let pt=CGPoint(x:a.x ?? 200,y:a.y ?? 200)
            for t in [CGEventType.leftMouseDown, .leftMouseUp] {
                guard let ev=CGEvent(mouseEventSource:source,mouseType:t,mouseCursorPosition:pt,mouseButton:.left) else { throw ProbeError("cannot allocate click event") }
                ev.post(tap:.cghidEventTap)
            }
        } else if a.kind=="wait" {
            // posts nothing: a baseline/record interval
        } else { throw ProbeError("unsupported action: \(a.kind)") }
    }
}
