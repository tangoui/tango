# Code editor demonstration

Root cause: the full live catalog follows AUTHORING §14 and ui-gallery, while
ui/editor's only application lives in examples/editor-demo. MarkdownEditor
is a separate component. Add editor.View/Model/State to the catalog and a
real editor.View gallery page, without touching ongoing editor-core work.

- [x] Add regression tests for an editable gallery editor, language detection/highlighting, undo/redo, read-only behavior and state across frames.
- [x] Implement normal and large-document scenes using existing Go editor APIs, with a bounded foreground highlighting budget and status text.
- [x] Bridge editor Tab/shortcuts with an explicit Escape/Tab exit path and an editor-only monospace font, keeping other preview fonts unchanged.
- [x] Add documentation and visible homepage/nav links; test actual WASM editing and catalog inclusion.
- [x] Build, check links and review before local integration.

Verification: the complete documentation suite has 21 passing tests, including
all 209 real WASM scenes and editor typing, exact multiline paste, undo/redo,
Command shortcuts and double-click word selection. Native tests cover four
classified language highlighters, read-only, retained file edits and large-file
scrolling. The gallery accessibility audit found missing names on the new
controls; the demo now supplies a named file selector and a named multiline
textbox with the read-only state. The full gallery suite and audit pass. Go overlay tests
and vet, VitePress build and 1,469 relative-link checks also pass. Catalog:
149 component entries, 209 scenes. Independent review found and verified the
Command modifier correction. Actual light/dark Go WASM pixel output was
inspected; real browser IME/clipboard/focus traversal remains unverified
because browser control was unavailable.

Integration target: local main, preserving concurrent editor/GPU/table changes.
Publishing follows the user's separate push instruction.
