package jsapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/sys/clipboard"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/sys/hotkey"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/notify"
	"github.com/tangoui/tango/sys/shell"
)

// Clipboard, Dialog, Notify, Shell, Menu and Hotkey modules.

var errCanceled = dialog.ErrCanceled

func oneString(fn func(string) (any, error)) Handler {
	return func(_ context.Context, _ *tango.Surface, args json.RawMessage) (any, error) {
		var v string
		if err := decode(args, &v); err != nil {
			return nil, err
		}
		return fn(v)
	}
}

func (api *API) registerSystem() {
	api.registerClipboard()
	api.registerDialogs()
	api.registerNotify()
	api.registerShell()
	api.registerMenu()
	api.registerHotkeys()
}

// ---- clipboard ----

func (api *API) registerClipboard() {
	a := api.app
	api.Handle(ClipboardRead, "clipboard.readText", noArgs(func() (any, error) { return clipboard.ReadText(a) }))
	api.Handle(ClipboardRead, "clipboard.readImage", noArgs(func() (any, error) {
		b, err := clipboard.ReadImage(a)
		if err != nil || b == nil {
			return nil, err
		}
		return base64.StdEncoding.EncodeToString(b), nil
	}))
	api.Handle(ClipboardRead, "clipboard.readFiles", noArgs(func() (any, error) {
		p, err := clipboard.ReadFiles(a)
		if p == nil {
			p = []string{}
		}
		return p, err
	}))
	api.Handle(ClipboardWrite, "clipboard.writeText", oneString(func(text string) (any, error) {
		return nil, clipboard.WriteText(a, text)
	}))
	api.Handle(ClipboardWrite, "clipboard.writeImage", oneString(func(b64 string) (any, error) {
		b, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, ipc.NewError(ipc.CodeBadArgs, "image must be base64-encoded PNG")
		}
		return nil, clipboard.WriteImage(a, b)
	}))
	api.Handle(ClipboardWrite, "clipboard.writeFiles", func(_ context.Context, _ *tango.Surface, args json.RawMessage) (any, error) {
		var paths []string
		if err := decode(args, &paths); err != nil {
			return nil, err
		}
		return nil, clipboard.WriteFiles(a, paths)
	})
}

// ---- dialogs ----

type messageJS struct {
	Kind    string   `json:"kind"` // info | warning | error | question
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Buttons []string `json:"buttons"`
}

type filterJS struct {
	Name     string   `json:"name"`
	Patterns []string `json:"patterns"`
}

type fileJS struct {
	Title         string     `json:"title"`
	Directory     string     `json:"directory"`
	Filename      string     `json:"filename"`
	Filters       []filterJS `json:"filters"`
	Multiple      bool       `json:"multiple"`
	Folders       bool       `json:"folders"`
	CanCreateDirs bool       `json:"canCreateDirs"`
	ShowHidden    bool       `json:"showHidden"`
}

func (f fileJS) options(parent *tango.Surface) dialog.FileOptions {
	o := dialog.FileOptions{
		Parent: parent, Title: f.Title, Directory: f.Directory, Filename: f.Filename,
		Multiple: f.Multiple, PickFolders: f.Folders, CanCreateDirs: f.CanCreateDirs, ShowHidden: f.ShowHidden,
	}
	for _, fl := range f.Filters {
		o.Filters = append(o.Filters, dialog.FileFilter{Name: fl.Name, Patterns: fl.Patterns})
	}
	return o
}

var messageKinds = map[string]dialog.Kind{
	"": dialog.Info, "info": dialog.Info, "warning": dialog.Warning, "error": dialog.Error, "question": dialog.Question,
}

func (api *API) registerDialogs() {
	a := api.app
	api.Handle(Dialogs, "dialog.message", func(_ context.Context, s *tango.Surface, args json.RawMessage) (any, error) {
		var m messageJS
		if err := decode(args, &m); err != nil {
			return nil, err
		}
		kind, ok := messageKinds[m.Kind]
		if !ok {
			return nil, ipc.NewError(ipc.CodeBadArgs, "unknown message kind "+m.Kind)
		}
		return dialog.Message(a, dialog.MessageOptions{Parent: s, Kind: kind, Title: m.Title, Message: m.Message, Buttons: m.Buttons})
	})
	api.Handle(Dialogs, "dialog.open", func(_ context.Context, s *tango.Surface, args json.RawMessage) (any, error) {
		var f fileJS
		if err := decode(args, &f); err != nil {
			return nil, err
		}
		paths, err := dialog.Open(a, f.options(s))
		if err != nil {
			return nil, err
		}
		if paths == nil {
			paths = []string{}
		}
		return paths, nil
	})
	api.Handle(Dialogs, "dialog.save", func(_ context.Context, s *tango.Surface, args json.RawMessage) (any, error) {
		var f fileJS
		if err := decode(args, &f); err != nil {
			return nil, err
		}
		return dialog.Save(a, f.options(s))
	})
}

// ---- notifications ----

type noteJS struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Body     string `json:"body"`
	Silent   bool   `json:"silent"`
}

func (api *API) registerNotify() {
	a := api.app
	api.Handle(Notify, "notify.requestPermission", noArgs(func() (any, error) { return notify.RequestPermission(a) }))
	api.Handle(Notify, "notify.send", func(_ context.Context, s *tango.Surface, args json.RawMessage) (any, error) {
		var n noteJS
		if err := decode(args, &n); err != nil {
			return nil, err
		}
		api.noteOnce.Do(func() { notify.OnClick(a, api.noteClicked) })
		id, err := notify.Send(a, notify.Message{Title: n.Title, Subtitle: n.Subtitle, Body: n.Body, Silent: n.Silent})
		if err != nil {
			return nil, err
		}
		if s != nil {
			api.mu.Lock()
			api.notes[id] = s
			api.mu.Unlock()
		}
		return id, nil
	})
}

// noteClicked tells the page that sent the notification, or every page
// when that one is gone.
func (api *API) noteClicked(id string) {
	api.mu.Lock()
	s := api.notes[id]
	api.mu.Unlock()
	data := map[string]string{"id": id}
	if s != nil {
		api.Emit(s, "notify.click", data)
		return
	}
	api.Broadcast("notify.click", data)
}

// ---- shell ----

func (api *API) registerShell() {
	a := api.app
	api.Handle(OpenURL, "shell.openURL", oneString(func(u string) (any, error) { return nil, shell.OpenURL(a, u) }))
	api.Handle(OpenPath, "shell.openPath", oneString(func(p string) (any, error) { return nil, shell.OpenPath(a, p) }))
	api.Handle(OpenPath, "shell.revealPath", oneString(func(p string) (any, error) { return nil, shell.RevealPath(a, p) }))
}

// ---- context menus ----

type menuItemJS struct {
	ID          string       `json:"id"`
	Type        string       `json:"type"` // normal | checkbox | radio | separator
	Label       string       `json:"label"`
	Accelerator string       `json:"accelerator"`
	Disabled    bool         `json:"disabled"`
	Checked     bool         `json:"checked"`
	Group       string       `json:"group"`
	Role        string       `json:"role"`
	Icon        string       `json:"icon"` // base64 PNG
	Submenu     []menuItemJS `json:"submenu"`
}

// pageRoles are the standard items a page may put in its menus: edit
// actions on its own content. App-wide roles (quit, hide, ...) would get
// around AppControl.
var pageRoles = map[string]menu.Role{
	"copy": menu.Copy, "paste": menu.Paste, "cut": menu.Cut, "selectAll": menu.SelectAll,
	"undo": menu.Undo, "redo": menu.Redo,
}

const maxMenuDepth = 8

// buildMenu converts the page's description; choose is called with the id
// of the item clicked.
func buildMenu(items []menuItemJS, depth int, choose func(id string)) (*menu.Menu, error) {
	if depth > maxMenuDepth {
		return nil, ipc.NewError(ipc.CodeBadArgs, "menu nested too deeply")
	}
	m := menu.New()
	for i, it := range items {
		id := it.ID
		if id == "" {
			id = strconv.Itoa(i)
		}
		click := func() { choose(id) }
		var item *menu.Item
		switch {
		case it.Type == "separator":
			m.Separator()
			continue
		case it.Role != "":
			r, ok := pageRoles[it.Role]
			if !ok {
				return nil, ipc.NewError(CodeForbidden, "menu role not allowed in page menus: "+it.Role)
			}
			item = m.Role(r)
			if it.Label != "" {
				item.Label = it.Label
			}
		case len(it.Submenu) > 0:
			sub, err := buildMenu(it.Submenu, depth+1, choose)
			if err != nil {
				return nil, err
			}
			item = m.Add(it.Label, nil)
			item.Submenu = sub
		case it.Type == "checkbox":
			item = m.Check(it.Label, it.Checked, click)
		case it.Type == "radio":
			g := it.Group
			if g == "" {
				g = "default"
			}
			item = m.Radio(g, it.Label, it.Checked, click)
		case it.Type == "" || it.Type == "normal":
			item = m.Add(it.Label, click)
		default:
			return nil, ipc.NewError(ipc.CodeBadArgs, "unknown menu item type "+it.Type)
		}
		item.Accelerator = it.Accelerator
		item.Disabled = it.Disabled
		if it.Icon != "" {
			if b, err := base64.StdEncoding.DecodeString(it.Icon); err == nil {
				item.Icon = b
			}
		}
	}
	return m, nil
}

type popupResult struct {
	ID      *string `json:"id"` // null when dismissed
	Checked bool    `json:"checked"`
}

func (api *API) registerMenu() {
	api.Handle(Menu, "menu.popup", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var items []menuItemJS
		var x, y int
		if err := decode(args, &items, &x, &y); err != nil {
			return nil, err
		}
		var chosen *string
		var checked bool
		var m *menu.Menu
		m, err := buildMenu(items, 0, func(id string) {
			chosen = &id
			checked = findChecked(m, items, id)
		})
		if err != nil {
			return nil, err
		}
		if err := menu.Popup(s, x, y, m); err != nil {
			return nil, err
		}
		return popupResult{chosen, checked}, nil
	}))
}

// findChecked reports the Checked state of the item with id after the
// click toggled it (main thread, inside the click).
func findChecked(m *menu.Menu, items []menuItemJS, id string) bool {
	for i, it := range items {
		if i >= len(m.Items) {
			return false
		}
		mi := m.Items[i]
		iid := it.ID
		if iid == "" {
			iid = strconv.Itoa(i)
		}
		if len(it.Submenu) > 0 && mi.Submenu != nil {
			if findChecked(mi.Submenu, it.Submenu, id) {
				return true
			}
			continue
		}
		if iid == id {
			return mi.Checked
		}
	}
	return false
}

// ---- global shortcuts ----

type hotkeyEvent struct {
	ID          int    `json:"id"`
	Accelerator string `json:"accelerator"`
}

func (api *API) registerHotkeys() {
	api.Handle(Hotkeys, "hotkey.register", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var accel string
		if err := decode(args, &accel); err != nil {
			return nil, err
		}
		if err := hotkey.Validate(accel); err != nil {
			return nil, err // bad_args (wireError)
		}
		ps := api.page(s)
		if ps == nil {
			return nil, errNoSurface
		}
		api.mu.Lock()
		ps.nextHK++
		id, gen := ps.nextHK, ps.gen
		api.mu.Unlock()
		h, err := hotkey.Register(api.app, accel, func() {
			api.Emit(s, "hotkey.fire", hotkeyEvent{id, accel})
		})
		if err != nil {
			return nil, err
		}
		api.mu.Lock()
		if api.surfaces[s] != ps || ps.gen != gen { // closed or reloaded meanwhile
			api.mu.Unlock()
			_ = h.Unregister()
			return nil, ipc.NewError(ipc.CodeCanceled, "page went away")
		}
		ps.hotkeys[id] = h
		api.mu.Unlock()
		return id, nil
	}))
	api.Handle(Hotkeys, "hotkey.unregister", windowHandler(func(s *tango.Surface, args json.RawMessage) (any, error) {
		var id int
		if err := decode(args, &id); err != nil {
			return nil, err
		}
		api.mu.Lock()
		var h *hotkey.Hotkey
		if ps := api.surfaces[s]; ps != nil {
			h = ps.hotkeys[id]
			delete(ps.hotkeys, id)
		}
		api.mu.Unlock()
		if h == nil {
			return nil, ipc.NewError(ipc.CodeNotFound, "no such hotkey")
		}
		return nil, h.Unregister()
	}))
}
