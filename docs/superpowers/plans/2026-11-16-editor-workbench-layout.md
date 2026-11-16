# Correct workbench layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix the observed workbench structure while retaining existing Ant Design UI.
**Architecture:** Existing document controller remains responsible for file safety; shell composes one native-integrated titlebar, activity rail, resizable explorer, editor tabs and status. A filesystem explorer loads one directory at a time and routes selections through the guarded open function.
**Tech Stack:** Go and existing Tango TitleBar/Splitter/Tree/Tabs.
**Spec:** docs/superpowers/specs/2026-11-16-editor-workbench-layout-design.md

## Global Constraints

- Existing Ant Design components and tokens; no public API changes.
- Keep live user processes and unsaved documents untouched.
- Native and custom close must retain dirty protection.
- Preserve editor shortcuts, search, diff, generated-file and performance modes.

## Review Focus

- Single titlebar and explorer/tab header alignment in real native windows.
- Directory expansion stays bounded; symlink directories cannot recurse.
- File selection respects Save/Don't Save/Cancel and restores selection on refusal.
- Sidebar resize and narrow-mode transitions preserve editor model/focus.
- The custom close button must not bypass Surface.OnClose.

### Task 1: Real filesystem navigation
- [ ] Add tests for sorted directories, lazy loading, symlinks, error preservation and guarded file selection.
- [ ] Observe failure; implement explorer.go and native Open Folder using existing dialog and guarded document opening.
- [ ] Run editor-demo tests.

### Task 2: Correct workbench composition
- [ ] Add Driver regressions for no settings form in sidebar, aligned explorer/tab rows, narrow code space and custom-frame close guard.
- [ ] Replace redundant menu row with existing TitleBar, macOS native traffic-light reservation, shared-height explorer/tab headers and existing Splitter sidebar.
- [ ] Render light/dark/narrow/split/tree states and inspect actual native preview; fix clipping/geometry.

### Task 3: Final verification
- [ ] Test editor package/full suite, build, vet, formatting and diff checks.
- [ ] Fresh read-only review of final diff and regressions for significant findings.
- [ ] Launch separate final preview and provide actual screenshot evidence. Leave unmerged until the corrected layout is reviewable.
