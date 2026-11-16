# Editor demo visual polish

## Intent and direction

Make the existing editor example feel like a coherent desktop code editor. Preserve all editing and file-safety behavior. The proposed direction is a compact IDE shell inspired by familiar code editors, using the project's existing Ant Design theme tokens and components.

Approved clarification: borrow only the VS Code layout. Continue using the existing Ant Design 6.7 components, typography, icons, radii, spacing scale, and light/dark/high-contrast tokens; do not introduce a VS Code visual theme.

## Approaches

- Recommended: compact menu, one active-document tab, collapsible document sidebar, code canvas, and a segmented status bar. This gives the existing capabilities a familiar home without adding a project manager.
- Minimal alternative: only a document tab and code canvas, with actions inside menus. Quieter, but less useful for demonstrating the available editor controls.
- Toolbar-only alternative: align the existing controls and adjust spacing. Smallest change, but retains the visual density that prompted the feedback.

## Proposed layout

- A compact File / Edit / View menu replaces permanent Open / Save / Save As / Find / Replace / Theme / Language controls. Keep shortcuts and descriptive accessible names.
- A single document tab shows a file-type icon, the real filename or Untitled, and the unsaved marker. A small set of useful icon actions sits at the trailing edge.
- An optional, collapsible sidebar lists only the real active document and editing options. Do not invent a file tree, multiple open documents, or project navigation the app does not implement.
- The editor canvas occupies the remaining space. Match the shell to the selected light, dark, or high-contrast editor theme; maintain readable contrast, consistent borders, and restrained spacing using existing tokens.
- The bottom status bar separates file information and editing position. Put language/indent/EOL on the right; move renderer and highlight diagnostics into a menu or details view.
- Find/replace becomes a compact editor-local panel so search results remain visible. Preserve wrapping navigation, replacement, and focus restoration.
- Default content becomes a short readable sample rather than repeated benchmark lines; explicit large-file generation flags remain available.

## Verification and scope

Only change the editor example and its documentation/tests. Test menu actions, focus restoration, unsaved markers, theme/split controls, and narrow-window layout. Render and inspect light and dark screenshots before launching the revised app. Keep the already-running editor process and user edits untouched during development.
