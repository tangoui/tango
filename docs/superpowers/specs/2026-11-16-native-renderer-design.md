# macOS native display-list backend

The user wants Tango's native-UI memory and CPU to approach NSGo rather than
stopping at the offscreen-widget retention fix. NSGo's macOS implementation uses
retained Core Animation content layers, redrawing visible changed content. Tango
currently owns a complete Metal render target and drawable/pipeline resources.
The latest default remains GPU; the existing CPU-first auto policy is opt-in.

## Evidence and selected approach

Diagnostic copies under `outputs/tango-renderer-optimization-20261116` isolated
full/trivial shaders and direct Metal clears. Simplifying the shader saved about
10 MB, but even a bounded direct clear/present client had a large process
footprint. A CoreGraphics/CALayer prototype supports the same counter/quotes/list
component code and has much lower footprint. An initial missing-text coordinate
bug was caught by image inspection and a failing ink test; its memory results
are invalid. Only corrected-text measurements may support a claim.

Approaches considered: shader/atlas tuning cannot remove most measured Metal
context cost; blanket software rasterization incurs full-frame pixel copies and
can raise CPU. Select a retained CALayer/CoreGraphics backend for ordinary
macOS/arm64 UI, preserving Metal for forced GPU and sustained expensive drawing.
This is an implementation choice within the user's requested performance work.
Do not change components, themes, public UI APIs, or move rendering into a helper
process to make process accounting look smaller.

## Architecture

- Add an internal native-glyph bridge registered by ui/text on Darwin. It borrows
  a CGContext for a synchronous glyph batch, exposes availability and ink bounds,
  uses the already-selected CoreText face and positions, and never retains the
  caller's context or buffers. No public Face or paint API is added. Missing,
  custom, disabled-engine, or color-font support takes the existing raster path.
- Add a macOS/arm64 native display-list renderer. Keep per-op CALayers across
  frames. Plain/rounded fills and borders use native layer/shape properties;
  text uses bounded content layers and grouped CoreText glyph drawing. Compare
  content fingerprints, not shared mutable glyph slices; retain a private glyph
  copy only for changed content. Frame-only movement must not rerasterize text.
- Initially retain a correct full-frame paint.CPU fallback for unsupported ops,
  complex clipping/transforms/opacity, images and backdrop blur. Never silently
  omit commands. Common shadows should use cached bounded CPU-rendered patches
  so ordinary buttons do not force the whole surface into software rasterization.
- The canvas owns a distinct CoreGraphics-hosted layer state. Do not reuse the
  Metal identity flag from the throwaway prototype. Repeated target acquisition,
  resize, scale change, Present(nil), close and switching to Metal are explicit.
  Preserve existing frame scheduling, input, IME and accessibility behavior.
- Default eligible macOS/arm64 utility windows (logical area ≤100,000) start native.
  Larger windows retain GPU until native sustained CPU/tail acceptance passes;
  the first measured native implementation traded CPU for memory, and the
  transaction consolidation remeasurement was blocked by screen lock. Explicit gpu/cpu keep their
  meanings; explicit auto preserves the existing policy; native forces this
  backend. Overlays/unsupported platforms keep the current renderer policy.
- Default-native may promote once to Metal after six rendered frames exceeding
  an 8 ms native drawing budget within 100 ms gaps. Check the previous frame's
  completed work including presentation. Cold/isolated frames do not promote.
  Promotion failure restores native presentation or correctly falls back to CPU,
  never strands an empty hosted layer. Explicit native does not promote.
- Release layers, delegates, native images/colors/paths and font references at
  close. Trim releases rebuildable caches and redraws. Scale/size changes cannot
  retain stale text or textures. Native APIs remain UI-thread-only; glyph owner
  cleanup follows the existing locks and KeepAlive ownership rules.

## Acceptance and tradeoffs

Correct output precedes benchmarking. Tests must cover visible positioned
Latin/CJK text, repeated text, changed text and reused glyph buffers, nonzero
origins, clips, scale 1/1.5/2, asymmetric corners, fallback completeness, native
lifetime/close/trim, renderer selection, promotion/failure and existing input.
Native edge antialiasing may follow CoreGraphics; existing CPU/Metal pixel tests
and tolerances must not be weakened. Internal glyph position/bounds tests compare
against existing glyph placement and raster ink so clipping cannot fake savings.

Matched-source measurements must include full text and all rows. Compare GPU,
native, and current NSGo for counter/quotes/list, memory after one minute, CPU,
WindowServer increment, callbacks and tail intervals. Do not ship a default path
that achieves memory reduction by introducing substantial sustained CPU/frame
regressions in these scenes. Verify native and development builds, API snapshots,
Linux/Windows cross compilation and the repository suite. Intel macOS keeps the
existing path until its native ABI is validated. The aim is a measured material
improvement, not an unsupported promise to equal every NSGo result.

## Continuation: native text surface costs

The user requested further optimization after the conservative rollout. Measure
current main (1274788) before attributing gains: it also includes newer font,
CPU clipping and lazy Metal resources. Retain local text backing dimensions
through small ink changes, shrink above four times current ink area, and count
that maximum capacity in the existing 64 MiB bitmap budget. Exact visible clips
remain independent of backing capacity. Adjacent native text commands may share
a surface only with overlapping row geometry, at most eight commands, and a
union area at most twice total ink area. Each part retains its original color,
clip and owned glyph positions; non-text paint is an ordering barrier. Movement
must reuse contents, mutable caller buffers must invalidate them, and all source
references must retire at close. Reject the change if matched CPU/tail or memory
measurements regress materially. No new public component APIs or appearance
changes are authorized by this implementation choice.
