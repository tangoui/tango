package main

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
)

var errEmptySearch = errors.New("editor-demo: search text is empty")

type documentSession struct {
	path         string
	model        *editor.Model
	savedVersion uint64
}

func newDocumentSession(path, text string, reg *monarch.Registry) *documentSession {
	m := editor.NewModel(path, text, reg)
	return &documentSession{path: path, model: m, savedVersion: m.Buffer().AltVersion()}
}

func loadDocument(path string, reg *monarch.Registry) (*documentSession, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return newDocumentSession(path, string(text), reg), nil
}

func (d *documentSession) dirty() bool {
	return d.model.Buffer().AltVersion() != d.savedVersion
}

func (d *documentSession) text() string {
	return d.model.Snapshot().String()
}

func (d *documentSession) saveTo(path string) error {
	if path == "" {
		return errors.New("editor-demo: empty save path")
	}
	if err := os.WriteFile(path, []byte(d.model.Snapshot().TextWithEOL()), 0o644); err != nil {
		return err
	}
	d.path = path
	d.savedVersion = d.model.Buffer().AltVersion()
	return nil
}

func replaceAll(st *editor.State, find, replacement string, caseSensitive bool) (int, error) {
	if find == "" {
		return 0, errEmptySearch
	}
	snap := st.Snapshot()
	text := snap.String()
	matches := make([][]int, 0)
	if caseSensitive {
		for from := 0; from <= len(text)-len(find); {
			i := strings.Index(text[from:], find)
			if i < 0 {
				break
			}
			i += from
			matches = append(matches, []int{i, i + len(find)})
			from = i + len(find)
		}
	} else {
		re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(find))
		if err != nil {
			return 0, err
		}
		matches = re.FindAllStringIndex(text, -1)
	}
	if len(matches) == 0 {
		return 0, nil
	}
	edits := make([]buffer.Edit, len(matches))
	for i, match := range matches {
		edits[i] = buffer.Edit{Start: match[0], End: match[1], Text: replacement}
	}
	if err := st.Edit(edits, editor.Selection{}); err != nil {
		return 0, err
	}
	return len(edits), nil
}

type discardAction uint8

const (
	discardCancel discardAction = iota
	discardSave
	discardDontSave
)

func resolveDiscard(dirty bool, action discardAction) bool {
	return !dirty || action != discardCancel
}

type documentController struct {
	registry *monarch.Registry
	theme    *theme.Theme
	split    bool
	wrap     bool
	session  *documentSession
}

func newDocumentController(reg *monarch.Registry, th *theme.Theme, split, wrap bool) *documentController {
	return &documentController{registry: reg, theme: th, split: split, wrap: wrap}
}

func (c *documentController) install(d *documentSession) {
	if background != nil {
		background.Close()
	}
	if st != nil {
		st.Close()
	}
	if second != nil {
		second.Close()
	}
	c.session = d
	background = newModelHighlight(d.model)
	st, second = editor.NewView(d.model, c.theme), nil
	if c.wrap {
		st.Options.WordWrap = editor.WrapOn
	}
	if c.split {
		second = editor.NewView(d.model, c.theme)
		second.Options.WordWrap = st.Options.WordWrap
	}
	active, focused, hlDone = st, false, false
}

func (c *documentController) replace(path string) error {
	d, err := loadDocument(path, c.registry)
	if err != nil {
		return err
	}
	c.install(d)
	return nil
}

func (c *documentController) save() error {
	if c.session == nil || c.session.path == "" {
		return errors.New("editor-demo: document has no path")
	}
	return c.session.saveTo(c.session.path)
}

func (c *documentController) saveAs(path string) error {
	if c.session == nil {
		return errors.New("editor-demo: no document")
	}
	return c.session.saveTo(path)
}

func (c *documentController) title() string {
	if c.session == nil {
		return "Tango Editor"
	}
	name := c.session.path
	if name == "" {
		name = "Untitled"
	} else {
		name = filepath.Base(name)
	}
	if c.session.dirty() {
		name = "*" + name
	}
	return "Tango Editor — " + name
}
