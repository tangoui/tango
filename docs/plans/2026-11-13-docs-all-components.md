# Full component demonstrations

- [x] Compile existing gallery demo files with the docs host using a generated Go overlay; expose registered pages and test source composition.
- [x] Parse the full AUTHORING table including qualified names, map every row to live demos, add missing demo/alias coverage, and fail on unmapped components.
- [x] Add searchable group navigation, per-component live pages, additional scenarios and scrolling; preserve the existing six guides.
- [x] Bundle licensed Chinese demo glyphs and document font provenance/reproduction.
- [x] Test catalog coverage, representative input, all live scenes, theme changes, static /tango/ paths, docs links, and review before local integration.

User request: all components demonstrated, extending the already approved Go/WASM design. No deployment/push in this change. Preserve unrelated main work.

Verified on the composed source: 148 component groups, 207 live scenarios, eight categories; all WASM scenes render and accept scroll. Native regression tests verify mobile state retention and reaching the bottom of bare dashboards; wheel tests preserve component horizontal scrolling. Source fonts cover Chinese demo labels. All component routes and runtime assets are checked under /tango/. Independent review findings were fixed with failing-then-passing regressions. Browser control remains unavailable; manual browser checks remain listed in the playground README.
