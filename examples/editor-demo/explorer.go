package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/icon/filetype"
	"github.com/tangoui/tango/ui/layout"
)

// Only directories the user expands are read. Directory symlinks stay leaves,
// so aliases cannot introduce cycles into the navigation tree.
type fileExplorer struct {
	root               string
	nodes              []ui.TreeNode
	expanded, selected []string
	err                string
}

func (e *fileExplorer) openFolder(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	nodes, err := readDirectory(path)
	if err != nil {
		return err
	}
	e.root, e.nodes, e.expanded, e.selected, e.err = path, nodes, nil, nil, ""
	return nil
}

func readDirectory(path string) ([]ui.TreeNode, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(entries, func(a, b os.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
	})
	nodes := make([]ui.TreeNode, 0, len(entries))
	for _, entry := range entries {
		nodes = append(nodes, ui.TreeNode{Key: filepath.Join(path, entry.Name()), Title: entry.Name(), IsLeaf: !entry.IsDir()})
	}
	return nodes, nil
}

func (e *fileExplorer) load(path string) bool {
	nodes, err := readDirectory(path)
	if err != nil {
		e.err = err.Error()
		return false
	}
	var update func([]ui.TreeNode) bool
	update = func(list []ui.TreeNode) bool {
		for i := range list {
			if list[i].Key == path {
				list[i].Children = nodes
				return true
			}
			if update(list[i].Children) {
				return true
			}
		}
		return false
	}
	update(e.nodes)
	return false // synchronous completion, not a spinner
}

func (e *fileExplorer) selectFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		e.err = err.Error()
		return false
	}
	if !info.Mode().IsRegular() {
		return false
	}
	if !openDocumentPath(path) {
		e.selected = nil
		if documents.session.path != "" {
			e.selected = []string{documents.session.path}
		}
		return false
	}
	e.selected = []string{path}
	return true
}

func openFolderDialog() {
	paths, err := dialog.Open(app, dialog.FileOptions{Parent: surface, Title: "Open Folder", PickFolders: true})
	if err != nil || len(paths) == 0 {
		return
	}
	if err := shellUI.explorer.openFolder(paths[0]); err != nil {
		showMessage(dialog.Error, "Open folder failed", err.Error())
		return
	}
	shellUI.sidebar = true
	restoreEditorFocus()
}

func renderExplorer(c *ui.Context, treeHeight float32) {
	th := c.Theme
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Text(c, "EXPLORER").Size(th.FontSizeSM).Strong().Secondary()
			ui.Spacer(c)
			shellIcon(c, icon.FolderOpen, "Open folder", func() { postFileAction(openFolderDialog) })
		}, layout.H(workbenchHeaderHeight(c)), layout.Middle(), layout.PadVH(0, th.PaddingSM)).Bg(th.BgContainer)
		renderExplorerContent(c, treeHeight)
	}, layout.Fill(), layout.MinW(0), layout.MinH(0)).Bg(th.BgLayout).Clip()
}

func renderExplorerContent(c *ui.Context, treeHeight float32) {
	th := c.Theme
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Icon(c, icon.Down).Size(th.FontSizeSM)
			ui.Text(c, "OPEN EDITORS").Size(th.FontSizeSM).Secondary()
		}, layout.Middle(), layout.Gap(th.PaddingXXS), layout.PadVH(th.PaddingXXS, th.PaddingXS))
		name := documentDisplayName(documents.session.path)
		if documents.session.dirty() {
			name += " •"
		}
		ic := filetype.ForFlavor(th.Flavor).Icon(shellDocumentFilename(), false, false)
		row := ui.Row(c, func() {
			ui.Icon(c, ic).Size(th.FontSizeLG).Color(th.Primary.Base)
			ui.Text(c, name).Size(th.FontSizeSM).NoWrap().Ellipsis().Layout(layout.Grow(1), layout.MinW(0))
		}, layout.H(th.ControlHeight), layout.Middle(), layout.Gap(th.PaddingXS), layout.PadVH(0, th.PaddingSM)).Bg(th.Primary.Bg)
		if row.Clicked() {
			restoreEditorFocus()
			shellUI.sidebarOverlay = false
		}
		ui.Div(c, layout.H(th.PaddingXS))
		if shellUI.explorer.root == "" {
			ui.Column(c, func() {
				ui.Text(c, "No folder opened").Size(th.FontSizeSM).Secondary()
				if ui.Button(c, "Open Folder").Small().Block().Clicked() {
					postFileAction(openFolderDialog)
				}
			}, layout.Pad(th.PaddingSM), layout.Gap(th.PaddingXS))
			return
		}
		e := &shellUI.explorer
		ui.Row(c, func() {
			ui.Icon(c, icon.Down).Size(th.FontSizeSM)
			ui.Text(c, strings.ToUpper(filepath.Base(e.root))).Size(th.FontSizeSM).Strong().Secondary().NoWrap().Ellipsis().Layout(layout.Grow(1), layout.MinW(0))
		}, layout.Middle(), layout.Gap(th.PaddingXXS), layout.PadVH(th.PaddingXXS, th.PaddingXS))
		tree := ui.Tree(c, e.nodes).Expanded(&e.expanded).Selected(&e.selected).ShowIcon().BlockNode().ExpandOnClick(true).LoadData(e.load).Height(max(120, treeHeight)).A11yLabel("Folder files").Show()
		if tree.SelectChanged() && len(e.selected) > 0 {
			path := e.selected[0]
			postFileAction(func() { e.selectFile(path) })
		}
		if e.err != "" {
			ui.Text(c, e.err).Size(th.FontSizeSM).Color(th.Error.Text).Ellipsis()
		}
	}, layout.Grow(1), layout.MinW(0), layout.MinH(0), layout.Gap(0)).Clip()
}
