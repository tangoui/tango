import Foundation

@main struct CoreTests {
    static func main() {
        precondition(hostNanoseconds(123, numer: 125, denom: 3) == 5125)
        precondition(hostNanoseconds(12, numer: 1, denom: 0) == nil)
        precondition(hostNanoseconds(UInt64.max, numer: 2, denom: 1) == nil)
        let body = Patch(x: 1, y: 1, width: 2, height: 1)
        var before = [UInt8](repeating: 0, count: 4 * 3 * 4)
        let p = cropPatch(before, width: 4, height: 3, patch: body)!
        before[0] = 255 // outside body: cursor or status
        precondition(cropPatch(before, width: 4, height: 3, patch: body) == p)
        before[(1 * 4 + 1) * 4] = 255
        let q = cropPatch(before, width: 4, height: 3, patch: body)!
        precondition(patchDistance(p, q)! > 0)
        precondition(patchDistance(p, p) == 0)
        precondition(cropPatch(before, width: 4, height: 3, patch: Patch(x: -1, y: 0, width: 2, height: 2)) == nil)
        precondition(patchDistance([], []) == nil)
        let target = WindowSnapshot(id: 2, rect: CGRect(x: 100,y: 100,width: 200,height: 100), alpha: 1)
        let overlap = WindowSnapshot(id: 3, rect: CGRect(x: 200,y: 120,width: 100,height: 50), alpha: 1)
        let outside = WindowSnapshot(id: 4, rect: CGRect(x: 0,y: 0,width: 20,height: 20), alpha: 1)
        let invisible = WindowSnapshot(id: 5, rect: overlap.rect, alpha: 0)
        precondition(occlusion([target, overlap], target: 2, region: target.rect) == .clear) // window behind
        precondition(occlusion([overlap, target], target: 2, region: target.rect) == .blocked) // same layer, in front
        precondition(occlusion([outside, invisible, target], target: 2, region: target.rect) == .clear)
        precondition(occlusion([overlap], target: 2, region: target.rect) == .missingTarget)
        precondition(captureRequiresUnobscuredDisplay(isolatedWindow: false))
        precondition(!captureRequiresUnobscuredDisplay(isolatedWindow: true))
        // Alignment: the measure pass may show the body shifted as a whole.
        func bars(_ w: Int, _ h: Int, _ xs: [Int]) -> [UInt8] {
            var a = [UInt8](repeating: 255, count: w * h * 4)
            for x in xs { for y in 2..<(h - 2) { let o = (y * w + x) * 4; a[o] = 20; a[o+1] = 20; a[o+2] = 20 } }
            return a
        }
        let ref = bars(40, 10, [5, 6, 13, 21, 22, 30])
        let cur = bars(40, 10, [7, 8, 15, 23, 24, 32]) // content moved right by 2
        let s1 = bestShift(refs: [ref], curs: [cur], widths: [40], heights: [10], maxShift: 4)!
        precondition(s1.dx == 2 && s1.dy == 0 && s1.distance == 0)
        precondition(bestShift(refs: [ref], curs: [ref], widths: [40], heights: [10], maxShift: 4)!.dx == 0)
        let blank = bars(40, 10, [])
        let s2 = bestShift(refs: [blank], curs: [blank], widths: [40], heights: [10], maxShift: 4)!
        precondition(s2.dx == 0 && s2.dy == 0) // ties prefer the smallest shift
        let other = bars(40, 10, [1, 2, 3, 4, 17, 18, 19, 36])
        precondition(bestShift(refs: [ref], curs: [other], widths: [40], heights: [10], maxShift: 4)!.distance > 1)
        precondition(bestShift(refs: [ref], curs: [cur], widths: [40], heights: [9], maxShift: 4) == nil) // size mismatch
        let moved = shifted(Patch(x: 10, y: 20, width: 4, height: 3, template: "t"), dx: 2, dy: -1)
        precondition(moved.x == 12 && moved.y == 19 && moved.template == "t")
        print("21 native core assertions passed")
    }
}
