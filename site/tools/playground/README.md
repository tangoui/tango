# Documentation playground

The full live catalog comes from `ui/AUTHORING.md` §14 (149 entries, including
qualified editor, chart and mobile names). Its component entries map to 209 base and
advanced scenarios registered by `examples/ui-gallery`; theme screenshot
variants use the preview's theme controls instead of duplicate navigation.
The six original tutorial pages remain available.

`/components/code-editor` demonstrates the real `ui/editor` View, Model and
State using Go, TypeScript, Python and JavaScript. The normal and 2,000-line
scenes retain each file's edits, highlight in bounded foreground batches,
and expose read-only, line numbers, undo/redo and restore controls. Editor
iframes register Go Mono before other fonts and forward clipboard paste
through the editor command so smart typing does not alter pasted code.
Tab indents; Escape followed by Tab exits the preview.

## Source and build

`build-playground.mjs` creates an ignored Go overlay mapping the original
`page_*.go` files and their helpers into the docs host package. It does not
move, maintain a copy of, or modify the native screenshot command. Screenshot
`Setup` hooks are never called by browser previews. The mobile screen factory
now retains its closure between frames so its controls keep edited state.

The build runs the composed package natively to enumerate registrations,
checks every AUTHORING row against real scene names, generates the catalog
and component pages, then compiles the same package for js/wasm. Missing
mappings stop the build. Composite APIs link to the real parent scenarios
that demonstrate them, with explicit aliases in `component-catalog.mjs`.

All Go components, layout, themes and pixels are real. `main_js.go` implements
`ui.Window`, forwarding CPU RGBA frames to Canvas and browser input to Go.
Long scenes use a Go scroll container; bare dashboards retain their authored
height. Horizontal gestures go to Go components; Shift+wheel pans the outer
canvas when it is wider than the viewport. Mouse and touch are separate input
paths, so touches are not synthesized twice.

Each iframe owns its Go globals. Gallery Reset reloads that iframe to reset
captured demo state as well as the Runner. The browser back/forward cache
pauses and restores the existing session.

## Commands

From the repository root:

```bash
npm --prefix site run docs:test
go test -overlay=site/.playground/gallery-overlay.json ./site/tools/playground
go vet -overlay=site/.playground/gallery-overlay.json ./site/tools/playground
go test ./examples/ui-gallery -run 'TestCodeEditor|TestMobileGalleryKeepsInteractiveStateAcrossFrames'
npm --prefix site run docs:build
npm --prefix site run docs:links
```

`docs:dev`/`docs:build` generate the manifests, full component pages, source
overlay, WASM and matching `wasm_exec.js` automatically. Go is required.
Generated output is ignored and the ordinary `go test ./site/tools/playground`
continues to test the basic demos, fonts, key mapping and scroll wrapper.

Tests instantiate the actual served WASM module under Node and render/scroll
every base and advanced scene, validate all component mappings, exercise
representative interactions, themes, resize/reset/unknown names, check the
compressed module byte-for-byte, and test pointer, wheel, IME and lifecycle
translations. Editor tests verify actual WASM typing, paste, word selection,
undo/redo and highlighting; native tests check all four languages, read-only,
retained file edits and large-file scrolling. Native tests also cover the
mobile gallery state regression and access to lower content of bare scenes.

## Assets and fonts

Modern browsers download `playground.wasm.gz` (about 8.7 MiB in this build)
and decode it with DecompressionStream. The loader detects already-decoded
responses as well. Browsers without that API use the uncompressed module.
Both payloads contain the complete catalog. Iframes load lazily and reuse
cached assets; no backend service is needed.

Go Regular supplies Latin text. A 422 KiB OFL-licensed Noto Sans SC subset
supplies the actual gallery/locale glyphs; see `fonts/README.md` and its
reproduction script. A local TTF/OTF can add more characters without uploading
it to a server. License notices accompany the playground's public assets,
including `EDITOR-NOTICES.txt` for the embedded language grammars and themes.

## Verification limits

The computer/browser control service was unavailable during implementation.
Actual Chrome/Safari, IME and clipboard smoke checks remain manual. The
canvas has keyboard entry/exit, a hidden IME textarea and a status summary;
a full DOM accessibility mirror is not implemented. System dialogs, native
windows and network actions retain the sample implementations; this is not
full browser-platform support for Tango applications.

- [ ] In Chrome/Safari, search and open all eight groups and several advanced scenes.
- [ ] Exercise fields, Select, table horizontal scroll, bare dashboard vertical scroll and Reset.
- [ ] Verify mobile dragging/pinch, IME, copy/paste, Tab exit and browser Back.
- [ ] Verify all six theme variants, narrow layouts and failed WASM downloads.
- [ ] In the code editor, verify IME, exact multiline paste, double-click selection and Escape then Tab exit.
