package editor

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/when"
	"github.com/tangoui/tango/ui/geom"
)

// The context menu (Monaco's ContextMenuController showing the
// MenuId.EditorContext menu of its MenuRegistry):
//
// A right click on the text (not the gutter) focuses the view and, unless
// it lands inside a selection, puts a single caret there; then, with
// Options.ContextMenu, the menu opens at the pointer. Its items are the
// ones registered for MenuEditorContext (RegisterMenuItem: the built-in
// items below, and any the application or a plugin adds globally)
// followed by the view's own (ViewW.MenuItems). An item is shown when its
// When holds in the view's context keys (contextkeys.go, with the view
// focused) and its command is registered; a submenu is shown when one of
// its items is. An item whose command's Precondition fails, or a Mutates
// command in a read-only view, is shown disabled (Monaco's
// MenuItemAction enablement). Items are sorted as Monaco's
// MenuInfo._compareMenuItems: the group "navigation" first, groups without
// a name last, the others by name; inside a group by Order, then by label.
// A separator goes between the groups that have shown items. The shortcut
// column shows the command's current keybinding from the view's keymap
// (ViewW.Keymap), as Monaco's ActionViewItem keybinding column.
//
// The menu is ui.ContextMenuAtWith, opened at the pointer (its top-right
// in right-to-left scopes) with no item highlighted, as Monaco's: focus
// moves to the menu, Down or Home highlights the first item (Enter does
// nothing until one is), the pointer or the arrow keys choose an item,
// Esc, a press outside or scrolling the view
// closes it, and focus returns to the view. The chosen command runs on the State as
// State.Execute with the item's Args, as if the view had focus.
//
// Built-in items (the commands of this package that Monaco 0.56's editor
// context menu offers; Monaco's group names and orders):
//
//	1_modification   Select All (editor.action.selectAll)                 order 1
//	                 Toggle Line Comment (editor.action.commentLine)      order 2, writable
//	9_cutcopypaste   Cut (editor.action.clipboardCutAction)               order 1, writable
//	                 Copy (editor.action.clipboardCopyAction)             order 2
//	                 Paste (editor.action.clipboardPasteAction)           order 4, writable
//	z_commands       Command Palette (editor.action.quickCommand)         order 1
//
// The Command Palette item is shown only when the application registers
// editor.action.quickCommand (this package has no palette).
//
// The view is built before the application's modifiers of the frame
// (ReadOnly, Keymap, Context, MenuItems), so the menu opened by a right
// click reflects the last frame's; the command chosen runs with the
// current ones.

// MenuEditorContext is the editor's context menu (Monaco's
// MenuId.EditorContext), for RegisterMenuItem.
const MenuEditorContext = "EditorContext"

// Monaco's group names of the editor context menu.
const (
	MenuGroupNavigation   = "navigation"     // first (go to definition, ...)
	MenuGroupModification = "1_modification" // edits of the text
	MenuGroupCutCopyPaste = "9_cutcopypaste" // the clipboard
	MenuGroupCommands     = "z_commands"     // the command palette
)

// MenuItem is an item of a menu (Monaco's IMenuItem / ISubmenuItem): a
// command, or a submenu of further items.
type MenuItem struct {
	// Command is the ID of the registered command the item runs. An item
	// whose command is not registered is not shown. Ignored when Submenu
	// is set.
	Command string
	// Label is the text shown; empty means the command's Label (its ID
	// when that is empty too).
	Label string
	// Group places the item: items are sorted by group ("navigation"
	// first, "" last, the others by name) with a separator between the
	// groups (Monaco's group, e.g. "9_cutcopypaste").
	Group string
	// Order sorts the items of a group (Monaco's order; then by label).
	Order float64
	// When is a when expression (package when) over the view's context
	// keys that must hold for the item to be shown; empty means always.
	When string
	// Args are the arguments the command runs with.
	Args any
	// Submenu makes the item a submenu of these items (sorted and grouped
	// the same way); it is shown when one of them is.
	Submenu []MenuItem
}

var menuReg = struct {
	sync.RWMutex
	m map[string][]MenuItem
}{m: map[string][]MenuItem{}}

// RegisterMenuItem adds item to the global menu named menu
// (MenuEditorContext for the editor's context menu), after the items
// already there (Monaco's MenuRegistry.appendMenuItem). It panics when the
// item has neither a command nor a submenu, or a When does not parse:
// those are programming errors. Items a view adds for itself go through
// ViewW.MenuItems.
func RegisterMenuItem(menu string, item MenuItem) {
	if menu == "" {
		panic("editor: RegisterMenuItem needs a menu")
	}
	if err := checkMenuItem(item); err != nil {
		panic(fmt.Sprintf("editor: menu %s: %v", menu, err))
	}
	menuReg.Lock()
	menuReg.m[menu] = append(menuReg.m[menu], item)
	menuReg.Unlock()
}

// checkMenuItem reports a malformed item (or item of its submenu).
func checkMenuItem(it MenuItem) error {
	if it.Command == "" && len(it.Submenu) == 0 {
		return fmt.Errorf("item %q has neither a command nor a submenu", it.Label)
	}
	if len(it.Submenu) > 0 && strings.TrimSpace(it.Label) == "" {
		return fmt.Errorf("a submenu needs a label")
	}
	if _, err := parseWhen(it.When); err != nil {
		return fmt.Errorf("item %q: when: %v", cmp.Or(it.Command, it.Label), err)
	}
	for _, s := range it.Submenu {
		if err := checkMenuItem(s); err != nil {
			return err
		}
	}
	return nil
}

// parseWhen parses a When; nil for an empty one.
func parseWhen(s string) (when.Expr, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	return when.Parse(s)
}

// menuItemsOf returns a copy of the items registered for menu.
func menuItemsOf(menu string) []MenuItem {
	menuReg.RLock()
	defer menuReg.RUnlock()
	return slices.Clone(menuReg.m[menu])
}

func init() {
	for _, it := range []MenuItem{
		{Command: "editor.action.selectAll", Group: MenuGroupModification, Order: 1},
		{Command: "editor.action.commentLine", Group: MenuGroupModification, Order: 2, When: writable},
		{Command: "editor.action.clipboardCutAction", Group: MenuGroupCutCopyPaste, Order: 1, When: writable},
		{Command: "editor.action.clipboardCopyAction", Group: MenuGroupCutCopyPaste, Order: 2},
		{Command: "editor.action.clipboardPasteAction", Group: MenuGroupCutCopyPaste, Order: 4, When: writable},
		{Command: "editor.action.quickCommand", Label: "Command Palette", Group: MenuGroupCommands, Order: 1},
	} {
		RegisterMenuItem(MenuEditorContext, it)
	}
}

// MenuItems adds items to this view's context menu, after the registered
// ones (MenuEditorContext): the application's or a plugin's items for this
// editor only. Call it every frame; the items of a frame that does not
// call it are gone. A When that does not parse hides its item.
func (w *ViewW) MenuItems(items ...MenuItem) *ViewW {
	w.vs.menu.host = append(w.vs.menu.host, items...)
	return w
}

// menuState is the context menu of a view (retained in viewState).
type menuState struct {
	open  bool               // a menu of this view is shown (ui.ContextMenuOpen)
	items []ui.MenuItem      // the items shown
	runs  map[string]menuRun // command of each item key
	// Items added with MenuItems this frame (host) and the last frame
	// (hostPrev): the menu opens before this frame's MenuItems calls.
	host, hostPrev []MenuItem
	// The last frame's ReadOnly, ContextMenu, keymap and Context function,
	// after its modifiers (seen: a frame finished).
	ro, on, seen bool
	km           *keymap.Keymap
	ctxFn        func(key string) (any, bool)
}

// menuRun is the command of a menu item.
type menuRun struct {
	cmd  string
	args any
}

// contextMenu opens the menu on a right click in the text (wheel: the
// view scrolled this frame, which closes the menu as Monaco hides its
// context view).
func (w *ViewW) contextMenu(wheel bool) {
	c, vs, id := w.c, w.vs, w.box.ID()
	m := &vs.menu
	m.hostPrev, m.host = m.host, m.hostPrev[:0]
	if m.open && wheel {
		ui.CloseContextMenu(c, id)
	}
	if p, ok := c.ContextMenuFor(id); ok && vs.text.Contains(p) {
		w.openMenu(p)
	}
	if m.open && !ui.ContextMenuOpen(c, id) {
		m.open, m.items, m.runs = false, nil, nil
	}
}

// openMenu handles a right click at window point p in the text.
func (w *ViewW) openMenu(p geom.Point) {
	c, st, vs := w.c, w.st, w.vs
	m := &vs.menu
	c.SetFocus(w.box.ID())
	if w.hasGeom() {
		// Monaco: the caret moves to the click unless it is inside a
		// selection.
		snap := st.Snapshot()
		off := w.offsetAt(snap, p)
		in := false
		for _, s := range st.sels() {
			if a, b := ordered(s); a <= off && off <= b {
				in = true
				break
			}
		}
		if !in {
			st.setSels([]Selection{{Anchor: off, Head: off}}, nil)
			vs.activity = c.Now()
		}
	}
	ro, on := w.opts.ReadOnly, w.opts.ContextMenu
	if m.seen {
		ro, on = m.ro, m.on
		// The keymap and the application's context keys too (labels,
		// platform keys, When and Precondition), for this build only.
		defer func(km *keymap.Keymap, fn func(string) (any, bool)) { w.km, w.ctxFn = km, fn }(w.km, w.ctxFn)
		w.km, w.ctxFn = m.km, m.ctxFn
	}
	if !on {
		return
	}
	b := menuBuild{
		ctx:  contextKeys{st: st, w: w, keyFocus: true, ro: ro, roSet: true},
		ro:   ro,
		km:   w.keymap(),
		runs: map[string]menuRun{},
	}
	src := append(menuItemsOf(MenuEditorContext), m.hostPrev...)
	items := b.items(src, "")
	if len(items) == 0 {
		ui.CloseContextMenu(c, w.box.ID())
		m.open, m.items, m.runs = false, nil, nil
		return
	}
	runs := b.runs
	m.open, m.items, m.runs = true, items, runs
	ui.ContextMenuAtWith(c, w.box.ID(), p, items, func(key string) {
		if r, ok := runs[key]; ok {
			vs.pend.menu = &r // run by finish, after the frame's modifiers
		}
	}, ui.ContextMenuOptions{NoAutoSelect: true})
}

// menuBuild turns MenuItems into the shown ui items of an opening menu.
type menuBuild struct {
	ctx  contextKeys
	ro   bool
	km   *keymap.Keymap
	runs map[string]menuRun
}

// items returns the shown items of src, sorted and grouped, keyed by
// their path ("2", "2.0") under prefix.
func (b *menuBuild) items(src []MenuItem, prefix string) []ui.MenuItem {
	type shown struct {
		it    MenuItem
		title string
		ui    ui.MenuItem
	}
	vis := make([]shown, 0, len(src))
	for i, it := range src {
		if e, err := parseWhen(it.When); err != nil || e != nil && !e.Eval(b.ctx) {
			continue
		}
		key := prefix + strconv.Itoa(i)
		if len(it.Submenu) > 0 {
			kids := b.items(it.Submenu, key+".")
			if len(kids) == 0 {
				continue
			}
			vis = append(vis, shown{it, it.Label, ui.MenuItem{Key: key, Label: it.Label, Children: kids}})
			continue
		}
		commands.RLock()
		rc, ok := commands.m[it.Command]
		commands.RUnlock()
		if !ok {
			continue
		}
		title := cmp.Or(it.Label, rc.Label, rc.ID)
		var short string
		if ks := b.km.Keys(it.Command, b.ctx); len(ks) > 0 {
			short = ks[0].Label(b.km.Platform())
		}
		enabled := (rc.pre == nil || rc.pre.Eval(b.ctx)) && !(rc.Mutates && b.ro)
		vis = append(vis, shown{it, title, ui.MenuItem{Key: key, Label: title, Shortcut: short, Disabled: !enabled}})
		b.runs[key] = menuRun{cmd: it.Command, args: it.Args}
	}
	slices.SortStableFunc(vis, func(x, y shown) int { return compareMenuItems(x.it, y.it, x.title, y.title) })
	out := make([]ui.MenuItem, 0, len(vis)+4)
	for i, s := range vis {
		if i > 0 && s.it.Group != vis[i-1].it.Group {
			out = append(out, ui.MenuItem{Divider: true})
		}
		out = append(out, s.ui)
	}
	return out
}

// compareMenuItems orders menu items as Monaco's
// MenuInfo._compareMenuItems: by group ("navigation" first, "" last, the
// others by name), then Order, then title (byte order where Monaco uses
// localeCompare).
func compareMenuItems(a, b MenuItem, ta, tb string) int {
	if a.Group != b.Group {
		switch {
		case a.Group == "":
			return 1
		case b.Group == "":
			return -1
		case a.Group == MenuGroupNavigation:
			return -1
		case b.Group == MenuGroupNavigation:
			return 1
		}
		if c := strings.Compare(a.Group, b.Group); c != 0 {
			return c
		}
	}
	if c := cmp.Compare(a.Order, b.Order); c != 0 {
		return c
	}
	return strings.Compare(ta, tb)
}
