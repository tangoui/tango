// Pure pixel and clock helpers, shared with the native probe's self-tests.
import Foundation
import CoreGraphics

struct WindowSnapshot {
    var id: UInt32
    var rect: CGRect
    var alpha: Double
}
enum Occlusion { case clear, blocked, missingTarget }

// A display crop includes whatever is composed above the target, so it needs
// a clear front-to-back order. A desktop-independent window capture contains
// only the target window; it can run beside other apps but is not physical
// display evidence.
func captureRequiresUnobscuredDisplay(isolatedWindow: Bool) -> Bool { !isolatedWindow }

// CGWindowListCopyWindowInfo returns front-to-back order. Layer numbers alone
// cannot distinguish a background window from a same-layer foreground window.
func occlusion(_ windows: [WindowSnapshot], target: UInt32, region: CGRect) -> Occlusion {
    guard let at = windows.firstIndex(where: { $0.id == target }) else { return .missingTarget }
    for w in windows[..<at] where w.alpha > 0 && w.rect.width > 1 && w.rect.height > 1 {
        if w.rect.insetBy(dx: -2, dy: -2).intersects(region) { return .blocked }
    }
    return .clear
}

struct Patch: Codable {
    var x: Int; var y: Int; var width: Int; var height: Int
    var template: String? = nil
}

func hostNanoseconds(_ ticks: UInt64, numer: UInt64, denom: UInt64) -> UInt64? {
    guard denom > 0 else { return nil }
    let (product, overflow) = ticks.multipliedReportingOverflow(by: numer)
    guard !overflow else { return nil }
    return product / denom
}

// Only the specified body patch is read; cursor/status changes elsewhere do
// not affect the score. Rows are packed BGRA after SCK's bytesPerRow is removed.
func cropPatch(_ pixels: [UInt8], width: Int, height: Int, patch: Patch) -> [UInt8]? {
    guard width > 0, height > 0, patch.x >= 0, patch.y >= 0,
          patch.width > 0, patch.height > 0, patch.x + patch.width <= width,
          patch.y + patch.height <= height, pixels.count == width * height * 4 else { return nil }
    var out: [UInt8] = []; out.reserveCapacity(patch.width * patch.height * 4)
    for y in patch.y..<(patch.y + patch.height) {
        let start = (y * width + patch.x) * 4
        out.append(contentsOf: pixels[start..<(start + patch.width * 4)])
    }
    return out
}

func patchDistance(_ a: [UInt8], _ b: [UInt8]) -> Double? {
    guard a.count == b.count, !a.isEmpty else { return nil }
    var sum: UInt64 = 0
    for i in a.indices where i % 4 != 3 { sum += UInt64(abs(Int(a[i]) - Int(b[i]))) }
    return Double(sum) / Double(a.count / 4 * 3)
}

// bestShift finds the whole-body shift (dx, dy), |dx|,|dy| <= maxShift, at
// which the current crops show the reference crops: cur[y+dy][x+dx] ==
// ref[y][x], as the mean absolute RGB difference over the overlap of all
// regions. Ties prefer the smallest shift, so a blank region stays put. nil
// when a crop does not have its region's size.
func bestShift(refs: [[UInt8]], curs: [[UInt8]], widths: [Int], heights: [Int], maxShift: Int) -> (dx: Int, dy: Int, distance: Double)? {
    guard refs.count == curs.count, refs.count == widths.count, refs.count == heights.count, !refs.isEmpty else { return nil }
    for i in refs.indices {
        let n = widths[i] * heights[i] * 4
        guard widths[i] > 2 * maxShift, heights[i] > 2 * maxShift, refs[i].count == n, curs[i].count == n else { return nil }
    }
    var best: (dx: Int, dy: Int, distance: Double)?
    for dy in -maxShift...maxShift {
        for dx in -maxShift...maxShift {
            var sum = 0, count = 0
            for i in refs.indices {
                let w = widths[i], h = heights[i], r = refs[i], c = curs[i]
                for y in max(0, -dy)..<min(h, h - dy) {
                    var o = (y * w + max(0, -dx)) * 4, q = ((y + dy) * w + max(0, -dx) + dx) * 4
                    for _ in max(0, -dx)..<min(w, w - dx) {
                        sum += abs(Int(r[o]) - Int(c[q])) + abs(Int(r[o+1]) - Int(c[q+1])) + abs(Int(r[o+2]) - Int(c[q+2]))
                        count += 3; o += 4; q += 4
                    }
                }
            }
            let d = count > 0 ? Double(sum) / Double(count) : .infinity
            if let b = best {
                let better = d < b.distance - 1e-9 || (abs(d - b.distance) <= 1e-9 && abs(dx) + abs(dy) < abs(b.dx) + abs(b.dy))
                if !better { continue }
            }
            best = (dx, dy, d)
        }
    }
    return best
}

// shifted moves a body patch by a whole-body shift found by bestShift.
func shifted(_ p: Patch, dx: Int, dy: Int) -> Patch {
    var q = p; q.x += dx; q.y += dy; return q
}
