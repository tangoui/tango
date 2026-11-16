# Complete editor demo design

## Intent

Turn `examples/editor-demo` into a complete desktop code editor example that a new Tango application can copy and run. It should demonstrate the editor's real workflow instead of only exercising rendering and performance paths.

## Goals

- Open one file from a native file picker and accept files opened by the operating system.
- Edit the document with the existing `ui/editor` view and background syntax highlighting.
- Save and Save As with native dialogs, preserving the file's current path and title.
- Show a clear dirty marker and ask before discarding unsaved changes when opening another file or quitting.
- Keep the current language selector, command palette, theme selection, split view, diff view, large-file generator, renderer switch, and scripted performance mode.
- Add a small find/replace toolbar backed by the editor's existing commands and selection behavior.

## Non-goals

- A production file browser, project/workspace management, tabs, or multi-document persistence.
- Replacing or extending the `ui/editor` public API.
- Implementing a second search engine or a new editor widget.
- Changing the existing editor or platform behavior outside this example.

## Approaches considered

1. **Extend the current `editor-demo` in place (chosen).** Reuse its model, highlighter, command palette, split/diff modes, and performance flags; add document lifecycle state around them. This keeps the example aligned with the existing editor API and avoids duplicating rendering code.
2. **Create a separate `examples/file-editor`.** This would isolate the workflow but duplicate setup and make the repository have two competing editor examples.
3. **Build a framework-level document controller.** This would support future products but expands the scope beyond an example and requires API design and cross-platform tests.

The first approach fits because the repository already treats `editor-demo` as the canonical editor example.

## Design

The demo owns a document session containing the path, display name, editor model, one or two views, highlighter lifetime, and the last saved snapshot version. A document is dirty when its current snapshot differs from the saved version. Opening a file replaces the session only after the dirty confirmation succeeds; Save writes the current snapshot using the detected line ending and updates the saved version; Save As changes the path after a successful write.

The toolbar adds Open, Save, Save As, Find, Replace, language, theme, split, and diff actions. Keyboard shortcuts use the editor command registry where available and route file actions through the demo's document controller. The native dialogs use `sys/dialog` and are invoked from the app's UI callbacks. `App.OnOpenFiles` and the window's file-drop handler both call the same open path.

The title includes `*` when dirty and the current file name. A close or replacement request with unsaved edits shows a three-button native message dialog: Save, Don't Save, Cancel. Generated documents remain untitled until Save As. Existing `-diff`, `-auto`, renderer, theme, language, split, wrap, and large-file flags continue to work.

## Verification

- `gofmt -l` on changed Go files.
- `go test ./examples/editor-demo` and `go test ./ui/editor/...`.
- `go build ./examples/editor-demo`.
- Manual smoke run with a real file, generated document, Save As, dirty discard, split view, and diff mode on macOS.
