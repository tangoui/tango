# Editor layout polish Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Use a familiar IDE layout while retaining Tango's existing Ant Design visual style and editor behavior.

**Architecture:** Extract the example's shell rendering from main.go into shell.go. The shell owns only presentation state and calls the existing file, editor, search, and palette actions. A cached Ant theme scoped with ConfigProvider keeps the shell consistent with the current editor theme across rebuilds.

**Tech Stack:** Go, existing Tango Menubar/Tabs/IconButton/Select/ConfigProvider, ui.Driver.

**Spec:** `docs/superpowers/specs/2026-11-16-editor-demo-polish-design.md`

## Global Constraints

- VS Code layout only; retain existing Ant Design 6.7 components, tokens, typography and icons.
- Preserve file lifecycle safety, shortcuts, language/theme/split controls, diff mode, explicit generated documents and performance flags.
- Use only real active-document data; no invented project tree or fake files.
- Keep the user's running editor and unsaved edits untouched.
- Changes stay in the example, its tests and documentation.

## Review Focus

- Narrow windows must retain access to actions and a usable code viewport.
- Theme changes inside a UI build must persist across subsequent builds.
- Menu/palette/search focus must return to the last active split pane.
- Document replacement and Save As must refresh labels without retaining old targets.
- Default sample must be readable while explicit generator flags keep their previous semantics.

### Task 1: Compact native-style shell

**Files:** Create `examples/editor-demo/shell.go`, `shell_test.go`; modify `main.go`, `document.go`.

**Interfaces:** `shellState` owns sidebar/dialog/tab/theme state. `renderShell(c *ui.Context)` composes menus, sidebar, active-document tab, shared editor panes and status. `runShellAction(key string)` delegates to the existing file/editor actions. `shellTheme(name string) *uitheme.Theme` caches the existing Ant themes.

- [ ] Write driver tests for real document tab/dirty marker, File/View actions, sidebar toggling, theme persistence, split focus, and viewport layout at 480×640 and 1200×800.
- [ ] Run those tests and observe missing-shell failures.
- [ ] Extract the existing editor view/palette code and implement the shell using existing Menubar, Tabs and token-based containers. Collapse the sidebar automatically at narrow widths, retain settings via menus, keep view IDs stable.
- [ ] Run `go test ./examples/editor-demo -count=1`; fix regressions and commit `feat(editor-demo): arrange existing UI as compact editor shell`.

### Task 2: Inline search and readable startup sample

**Files:** Modify `search.go`, `search_test.go`, `main.go`; create `sample.go`, `sample_test.go`.

**Interfaces:** `searchPanel.render` becomes an editor-local panel and `showSearch(replace bool)` requests query focus. `startupDocument(file string, generatedLines int, explicitGenerator bool) (name, content string, err error)` preserves explicit file/generator behavior and otherwise returns a short TypeScript sample.

- [ ] Write tests that find remains inside the editor pane, query Enter/previous/replace/close work and return focus, the default document is short, and explicit generator/file flags still work.
- [ ] Observe failures, implement with existing Input/Button/IconButton controls and keep replacement/navigation helpers unchanged.
- [ ] Run `go test ./examples/editor-demo -count=1` and commit `feat(editor-demo): add inline search and readable welcome sample`.

### Task 3: Visual QA, documentation and review

**Files:** Update example documentation and tests; save screenshots in the plan workspace.

- [ ] Render light/dark/high-contrast, narrow, split, dirty and search states with ui.Driver; inspect screenshots and correct clipping/contrast issues.
- [ ] Run `go test ./...`, `go vet ./examples/editor-demo`, build, gofmt and `git diff --check`.
- [ ] Request one fresh read-only review of the whole branch; fix significant findings with regressions.
- [ ] Launch the polished example separately after verification, preserving the existing process. Commit documentation and provide screenshot evidence and run command.
