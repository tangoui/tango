# UI node lifetime fix

The memory investigation measured VirtualList footprint growing from 169 MB to
273 MB over one minute. A single-variable diagnostic replacing 16-object blocks
with separate objects kept the same scene near 149 MB. Forced GC confirmed that
old objects remain reachable rather than merely awaiting collection.

The user requested the fix after reviewing that diagnosis. This change will
allocate pointer-containing node, Label and Box objects independently, retaining
the existing reuse by widget ID. Keeping a live widget must not pin unrelated
widgets that have scrolled away. Existing externally retained widget pointers
must remain valid; no recycling or clearing externally reachable objects.

Approaches considered: clearing abandoned block slots could retain batching but
would mutate objects that a caller may still hold. Grouping by row would still
couple lifetimes and would not cover other dynamic trees. Independent allocations
give each object its actual lifetime and are the selected implementation.

Acceptance: weak-reference regressions demonstrate collection of abandoned
allocation peers and offscreen VirtualList widgets; existing identity, input,
layout and pointer lifetime tests pass; one-minute native-window measurements
show bounded retained heap; build/scroll benchmarks quantify allocation and CPU
cost. Theme, appearance, public API and default renderer behavior stay the same.

The separate Metal fixed-cost issue is investigated with isolated controls.
Only a verified cause will lead to a Metal change; renderer fallback is not a
substitute for fixing node lifetime.
