package main

import (
	"errors"
	"os"
	"regexp"
	"strings"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
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
