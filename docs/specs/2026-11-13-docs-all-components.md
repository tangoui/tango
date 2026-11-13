# Full live component catalog

Extend the existing VitePress / Go WASM preview to every row of ui/AUTHORING.md section 14, including chart.* and mobile.* entries. Keep actual Go component behavior, themes and static GitHub Pages hosting. Existing six detailed docs remain.

Reuse ui-gallery demo source at build time through Go's overlay support, without moving or hand-copying its source tree and without changing the native screenshot workflow. Generate a catalog mapping every component row to real registered demos, with explicit aliases for composite names, searchable grouped navigation, source links, and generated component pages. Fail the build for unmapped entries rather than silently showing Button or screenshots. Long demos scroll; additional gallery scenarios remain selectable. Reset reloads isolated runtimes to clear original gallery globals.

The existing font does not cover Chinese gallery text. Include a licensed subset of Noto Sans SC containing the actual demo/locale glyphs; retain local font import for additional glyphs. Tests cover mapping completeness, native/WASM rendering across the catalog, representative interactions, themes, scrolling dimensions and unknown IDs. Native-only actions use the demo's existing sample data; browser support is limited to UI previews.

Audit result: 148 component rows across eight groups map to 207 real base/advanced scenes. The old parser omitted qualified chart/mobile names. Added a regression fix for mobile gallery factories that recreated state each frame. Source overlay avoids a large source-tree move. Chinese demo glyphs add 422 KiB; gzip transport reduces the complete module to about 7.8 MiB. Real browser interaction remains pending because the browser control service is unavailable.
