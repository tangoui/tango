// Synthetic calibration window only; never reads user documents.
import AppKit

final class CalibrationView: NSView {
    var tick = 0
    override func draw(_ rect: NSRect) {
        (tick % 2 == 0 ? NSColor(calibratedRed: 0.8, green: 0.1, blue: 0.2, alpha: 1) : NSColor(calibratedRed: 0.1, green: 0.7, blue: 0.3, alpha: 1)).setFill()
        bounds.fill()
    }
}
@main struct Calibration {
    static func main() {
        let app=NSApplication.shared
        app.setActivationPolicy(.regular)
        let win=NSWindow(contentRect:NSRect(x:100,y:100,width:640,height:320),styleMask:[.titled],backing:.buffered,defer:false)
        win.title="Tango capture calibration"
        let view=CalibrationView(frame:NSRect(x:0,y:0,width:640,height:320))
        win.contentView=view; win.makeKeyAndOrderFront(nil); app.activate(ignoringOtherApps:false)
        let timer=Timer.scheduledTimer(withTimeInterval:1.0/30,repeats:true) { _ in view.tick += 1; view.needsDisplay=true }
        RunLoop.main.add(timer,forMode:.common)
        DispatchQueue.main.asyncAfter(deadline:.now()+10) { timer.invalidate(); app.terminate(nil) }
        app.run()
    }
}
