# Terminal workspace example implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a runnable native terminal workspace with independent tabs and split PTYs.

**Architecture:** Keep application-owned sessions in a small workspace model with an injected session factory. Render keyed trees using the existing Tabs, Splitter and terminal.View. The main package owns shell configuration, host shortcuts, application lifecycle and cleanup.

**Tech Stack:** Go 1.27, Tango self-drawn UI, sys/pty; no new dependencies.

**Spec:** `docs/superpowers/specs/2026-11-15-terminal-workspace-design.md`

## Global Constraints

- Add `examples/terminal-workspace`; retain the existing terminal example.
- Reuse existing Ant theme tokens and components; do not change shared UI packages.
- Maximum 12 tabs and 4 panes per tab; stable identities survive switching.
- Real local PTYs; background output continues without resizing hidden tabs.
- Close all owned sessions on removal/shutdown; launch failure preserves prior state.

## Review Focus

- A failed split must leave the old pane/tree usable (Task 1).
- Closing an inactive tab must not change the selected session (Task 1).
- Switching tabs must not forward queued input to the newly selected PTY (Task 2).
- A background shell exiting must retain its output and not quit the app (Task 3).
- Narrow windows and theme changes must keep controls and terminal text legible (Task 2).

### Task 1: Workspace/session ownership

**Files:** Create `examples/terminal-workspace/workspace.go`, `workspace_test.go`.

**Interfaces:** Produce `workspace` with `newTab() error`, `split(splitAxis) error`, `closePane()`, `closeTab(string)`, `selectTab(string)`, `cycleTab(int)`, `cyclePane()`, `activeTab() *terminalTab`, `activeSession() *session`, `close()`. An injected `openSession func(id string) (*session, error)` creates terminals. `terminalTab` owns root and active pane ID; `splitNode` owns stable ID, axis and children or one session.

- [ ] Write failing tests for independent sessions, stable selection, nested split/close collapse, failed factory rollback, limits and exactly-once cleanup.
- [ ] Run `go test ./examples/terminal-workspace`; expected: failures for the missing workspace behavior.
- [ ] Implement the bounded workspace model and session cleanup.
- [ ] Run `go test ./examples/terminal-workspace`; expected: PASS.
- [ ] Commit the model and tests.

### Task 2: Keyed UI and host entry point

**Files:** Create `examples/terminal-workspace/main.go`, `page.go`, `page_test.go`, `tan.json`.

**Interfaces:** Consume Task 1 workspace. Produce `terminalApp.page(*ui.Context)`, local PTY factory and host shortcut registration; UI actions call the workspace model and request focus on the current pane.

- [ ] Write headless driver tests for new tab, splitting, close, selection/focus, stale input on tab switch, hidden-pane size retention and light/dark render.
- [ ] Run the focused tests; expected: failures for the missing page behavior.
- [ ] Implement the page, options/flags, error/empty/exited states, shortcuts and lifecycle. Keep delayed shell-exit notifications on the app thread.
- [ ] Run `go test ./examples/terminal-workspace ./ui/terminal/... ./sys/pty`; expected: PASS.
- [ ] Commit the runnable UI and tests.

### Task 3: Native acceptance and usage

**Files:** Create `examples/terminal-workspace/e2e_test.go`, `README.md`; modify root `README.md` example table.

**Interfaces:** Consume the complete runnable example, its accessible controls, and Tango's existing automation harness.

- [ ] Write a native PTY acceptance scenario for independent commands, three tab-switch cycles, resizing, split lifecycle, shell exit retention and continued use of surviving sessions.
- [ ] Run `TANGO_E2E=1 go test ./examples/terminal-workspace -run TestWorkspaceNative -v`; expected: PASS after implementing any behavior exposed by the scenario.
- [ ] Document run commands, shortcut mapping, lifecycle and M1 compatibility limits; build `go build ./examples/terminal-workspace` and run `go test ./...`; expected: build success, record every suite failure.
- [ ] Commit the tests and usage docs; request one read-only whole-change review and fix important findings with regression tests.
