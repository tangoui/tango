# Live UI documentation implementation plan

Goal: run six real Go UI component demos inside VitePress on static GitHub Pages.
Spec: ../specs/2026-11-13-docs-live-playground.md

- [x] Add demo state/page code under site/tools/playground and verify interaction with ui.Driver tests before implementing pages.
- [x] Add a js/wasm ui.Window host and JavaScript Canvas/input transport. Verify Go WASM compilation and browser-independent event mapping tests.
- [x] Add a deterministic build script producing playground.wasm and Go's matching wasm_exec.js; wire into docs builds and ignore generated output.
- [x] Embed lazy, isolated previews in VitePress; document Button/Input/Switch/Slider/Select/Tabs with usage, options, events and code. Add navigation and gallery links.
- [x] Build and check the site under /tango/; verify interactive demos in an available browser, document any concrete verification limits, and review the diff.

Constraints: no changes to ui/ or platform backends; no server dependency; reset recreates state; loads honor the Pages base URL; rendering uses registered fonts; visible loading/errors; Tab navigation can leave the frame.

Verification: native Driver interaction tests and all 36 component/theme render combinations passed; actual Go WASM input/theme/reset/resize regression passed under Node; JavaScript input and BFCache lifecycle tests passed; VitePress /tango/ build and relative link checks passed. Review found and fixed Space interception and BFCache disposal bugs. Browser control service is unavailable, so an actual browser click/IME/clipboard smoke test remains pending. Homepage was also updated, per the user, to lead with Go GUI, design themes, a genuine gallery screenshot and live demos.
