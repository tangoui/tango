# Correct editor workbench layout

The user rejected the overall layout and explicitly asked the implementer to inspect and decide. Preserve Ant Design UI and stop repeated design questions.

Observed defects: native title plus a redundant branded menu row; a settings form occupies the navigation sidebar; explorer header and file tab are unrelated; no functioning file navigation.

Implement one integrated existing TitleBar, retaining native macOS traffic lights through TitleBarHidden; compact File/Edit/View menus and a command entry belong inside that bar. Below it, a narrow activity rail precedes a resizable navigation sidebar and the editor workspace. The explorer heading aligns with the file-tab strip; the sidebar contains current-document navigation and a real lazy filesystem tree, not persistent theme/language/settings fields. Put settings in the existing modal only. A native Open Folder action sets the actual navigation root; start with the real working folder. Keep current document safety when navigating files. Do not fabricate tabs, files or Git state.

Use existing TitleBar, Splitter, Tree, Tabs, icons and all Ant visual tokens. Keep native close and custom titlebar close under unsaved-change protection. Keep the code viewport large and settings reachable at 480px. Preserve current processes/edits; launch a separate preview to inspect the actual window before declaring success.
