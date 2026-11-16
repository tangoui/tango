# Tango Editor

A complete desktop code-editor example using Tango's existing Ant Design UI. The layout groups actions into File/Edit/View menus, one active-document tab, a collapsible sidebar, the code canvas and a compact status bar. The sidebar follows the current document; it does not simulate a project browser or multiple open files.

Run from the repository root:

```sh
go run ./examples/editor-demo
```

The default document is a short TypeScript welcome sample. It remains untitled until **File → Save As**. Open a real file, use a light/high-contrast theme, or start with two synchronized panes:

```sh
go run ./examples/editor-demo -file examples/editor-demo/sample.go
go run ./examples/editor-demo -theme vs
go run ./examples/editor-demo -theme hc-black -split -wrap
```

## Workflow

- **File:** Open, Save and Save As use native dialogs. Opening another file or quitting asks before discarding unsaved edits. Dropped files and files opened by the operating system follow the same workflow.
- **Edit:** Undo/Redo, inline Find/Replace and the command palette operate on the last focused editor pane. Enter in Find selects the next result. Escape or the close icon closes the search panel and restores editor focus.
- **View:** Toggle the sidebar, split view and word wrapping; choose an existing light, dark or high-contrast theme. In narrow windows the sidebar opens as the existing Drawer component, while settings also remain reachable from the menu.
- **Document tab:** Shows the current filename and a dot for unsaved edits. Save As refreshes the name. Both split panes share one document and undo history.
- **Status bar:** Shows caret position, indentation, EOL and language. Choose the language item to open settings. **Help → Editor details** contains renderer and highlighting diagnostics.

Keyboard shortcuts use Cmd on macOS and Ctrl on Windows/Linux:

| Action | Shortcut |
| --- | --- |
| Open | Cmd/Ctrl+O |
| Save | Cmd/Ctrl+S |
| Save As | Cmd/Ctrl+Shift+S |
| Find | Cmd/Ctrl+F |
| Replace | Cmd/Ctrl+H |
| Command palette | Cmd/Ctrl+Shift+P |

## Existing demonstrations

Diff mode remains available as a separate view. Explicit generation and scripted performance flags retain their original behavior:

```sh
go run ./examples/editor-demo -diff old.go new.go
go run ./examples/editor-demo -diff -inline -fold old.go new.go
go run ./examples/editor-demo -gen-lines 1000000
go run ./examples/editor-demo -auto -renderer cpu -quit-after 20s
```

For visual QA, set `TANGO_EDITOR_SCREENSHOTS` to a directory and run `go test ./examples/editor-demo -run TestShellVisualQA -count=1`. This renders the real page in light, dark, high-contrast, narrow, split, dirty and search states. Ordinary test runs do not write screenshots.
