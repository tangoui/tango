# Complete editor demo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend `examples/editor-demo` into a complete desktop code editor workflow with native file lifecycle actions, dirty-state protection, and find/replace while preserving the existing editor demonstrations.

**Architecture:** Keep the existing `main.go` rendering and performance paths, and add a focused document/session controller plus small pure helpers for file I/O, dirty tracking, and replacement. UI callbacks call the controller from the app's main thread; native dialogs use `sys/dialog`; the editor model remains the source of truth for text and undo history.

**Tech Stack:** Go, Tango `ui/editor`, `uihost`, `sys/dialog`, native file/message dialogs, Go tests.

**Spec:** `docs/superpowers/specs/2026-11-15-editor-demo-complete-design.md`

## Global Constraints

- Do not change the `ui/editor` public API.
- Keep the existing `-diff`, `-auto`, renderer, theme, language, split, wrap, and generated-large-document flags working.
- Use the editor model's LF-normalized snapshot and `Snapshot.TextWithEOL()` when writing files.
- File and message dialogs run from UI callbacks on the app's main thread.
- A dirty document must be protected before replacement or close: Save, Don't Save, or Cancel.

## Review Focus

- CRLF files: saving preserves the snapshot's detected EOL; test in the document helper.
- Undo back to the saved content: dirty state follows `Snapshot.Version`, not a one-way boolean; test in the document helper.
- Save As cancellation or write failure: the old path and dirty state remain unchanged; test the controller helper.
- Replacing a dirty document: Cancel leaves the current model and title untouched; test the lifecycle decision helper.
- Replace-all matches overlapping or empty strings: replacement rejects empty search text and applies non-overlapping edits from the current snapshot; test the pure replacement helper.

### Task 1: Document session and file persistence

**Files:**
- Create: `examples/editor-demo/document.go`
- Create: `examples/editor-demo/document_test.go`
- Modify: `examples/editor-demo/main.go:1-80` to use the session type for path/title/dirty state.

**Interfaces:**
- Produces `type documentSession struct { path string; model *editor.Model; savedVersion uint64 }`.
- Produces `func newDocumentSession(path, text string, reg *monarch.Registry) *documentSession`.
- Produces `func loadDocument(path string, reg *monarch.Registry) (*documentSession, error)`.
- Produces `func (d *documentSession) dirty() bool`, `func (d *documentSession) text() string`, and `func (d *documentSession) saveTo(path string) error`.
- Produces `func replaceAll(st *editor.State, find, replacement string, caseSensitive bool) (int, error)` for the UI task.

- [ ] **Step 1: Write failing unit tests** for loading, EOL-preserving save, dirty tracking after edit/undo, Save As failure preserving the old path, and replace-all rejecting an empty query.
- [ ] **Step 2: Run `go test ./examples/editor-demo -run 'Test(Document|ReplaceAll)' -count=1`** and verify the new tests fail because the session helpers do not exist.
- [ ] **Step 3: Implement `documentSession`** with `os.ReadFile`, `os.WriteFile`, `editor.NewModel`, `Snapshot.Version`, and `Snapshot.TextWithEOL`; only update `path` and `savedVersion` after a successful write.
- [ ] **Step 4: Implement `replaceAll`** by collecting non-overlapping byte ranges from the current snapshot, applying them through `State.Edit`, and returning the replacement count.
- [ ] **Step 5: Run the focused tests** and verify all pass.
- [ ] **Step 6: Commit** with `feat(editor-demo): add document persistence helpers`.

### Task 2: File lifecycle, dialogs, and dirty protection

**Files:**
- Modify: `examples/editor-demo/main.go` lifecycle globals, `setupDocument`, `main`, and `page`.
- Modify: `examples/editor-demo/document.go` for session replacement and title helpers.
- Test: `examples/editor-demo/document_test.go` for the pure discard decision.

**Interfaces:**
- Adds a `documentController` around the active session, active/secondary views, highlighter lifetime, current surface, and the existing diff mode.
- Adds `func (c *documentController) replace(path string) error`, `func (c *documentController) save() error`, and `func (c *documentController) saveAs(path string) error`.
- Adds `func (c *documentController) title() string` returning `*` before the basename when dirty.

- [ ] **Step 1: Add a pure `discardDecision` test** covering Save, Don't Save, and Cancel outcomes without opening a dialog.
- [ ] **Step 2: Run the focused test** and verify it fails before the controller exists.
- [ ] **Step 3: Refactor `setupDocument` into the controller** so every replacement closes the old highlighter/views only after the new file has loaded successfully.
- [ ] **Step 4: Wire `sys/dialog.Open`, `sys/dialog.Save`, and `sys/dialog.Message`** to Open, Save, Save As, and dirty replacement/close flows; use `Surface.OnClose` to veto close on Cancel and `Surface.SetTitle` after every path or dirty change.
- [ ] **Step 5: Wire `App.OnOpenFiles` and `Surface.OnFilesDropped`** to the same open-path flow, ignoring directories and reporting read/write errors through a message dialog.
- [ ] **Step 6: Preserve existing command-line startup and diff mode**; file lifecycle actions are disabled or hidden while `-diff` is active.
- [ ] **Step 7: Run `go test ./examples/editor-demo -run 'Test(Document|Discard)' -count=1` and `go build ./examples/editor-demo`**.
- [ ] **Step 8: Commit** with `feat(editor-demo): add native file lifecycle`.

### Task 3: Complete editor toolbar and find/replace UI

**Files:**
- Create: `examples/editor-demo/search.go`
- Create: `examples/editor-demo/search_test.go`
- Modify: `examples/editor-demo/main.go` toolbar and modal layout.

**Interfaces:**
- Adds `type searchPanel struct { open bool; query string; replacement string; caseSensitive bool; status string }`.
- Adds `func (p *searchPanel) render(c *ui.Context, target *editor.State) bool`.
- Adds toolbar actions for Open, Save, Save As, Find, Replace, theme, language, split, and the existing command palette.

- [ ] **Step 1: Write tests** for case-sensitive and case-insensitive replacement counts and for a no-match status.
- [ ] **Step 2: Run `go test ./examples/editor-demo -run TestReplaceAll -count=1`** and verify the new cases fail.
- [ ] **Step 3: Implement the search panel** with `ui.Input`, `ui.Checkbox`, Find Next/Previous buttons using the editor's selection commands, and Replace/Replace All using `replaceAll`.
- [ ] **Step 4: Add keyboard-friendly toolbar actions** that restore editor focus after a dialog/panel closes and update the dirty title/status immediately.
- [ ] **Step 5: Run `go test ./examples/editor-demo -count=1` and `gofmt -l examples/editor-demo/*.go`**; expect no test failures and no formatted-file output.
- [ ] **Step 6: Commit** with `feat(editor-demo): add search and file actions`.

### Task 4: Documentation and end-to-end verification

**Files:**
- Modify: `examples/editor-demo/main.go` command comment with the new shortcuts and workflow.
- Modify: `README.zh-CN.md` and `README.md` example table or editor section if the current documentation needs the complete workflow called out.

- [ ] **Step 1: Document run commands** for opening a real file, opening generated content, using Save As, and diff mode.
- [ ] **Step 2: Run `go test ./examples/editor-demo`, `go test ./ui/editor/...`, and `go build ./examples/editor-demo`**.
- [ ] **Step 3: Run a macOS smoke check** with a temporary text file: open, edit, save, Save As, cancel a dirty replacement, open a dropped file, and quit through the close box.
- [ ] **Step 4: Review `git diff --check` and the final diff** to ensure only the example, its tests, and documentation changed.
- [ ] **Step 5: Commit** with `docs(editor-demo): document the complete workflow`.
