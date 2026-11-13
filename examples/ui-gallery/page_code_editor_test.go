package main

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestCodeEditorDemoEditsUndoRedoAndReadOnly(t *testing.T) {
	demo := newCodeEditorDemo(false)
	defer demo.close()
	d := ui.NewDriver(demo.render, ui.DriverOptions{Width: 1000, Height: 560})
	defer d.Runner().Close()
	d.Frame()
	st := demo.activeState()
	original := st.Snapshot().String()
	d.Type("hello")
	if st.Snapshot().String() == original {
		t.Fatal("editor did not receive committed text")
	}
	edited := st.Snapshot().String()
	if !st.Execute("undo", nil) {
		t.Fatal("undo did not run")
	}
	d.Frame()
	if st.Snapshot().String() != original {
		t.Fatal("undo did not restore the document")
	}
	if !st.Execute("redo", nil) {
		t.Fatal("redo did not run")
	}
	d.Frame()
	if st.Snapshot().String() != edited {
		t.Fatal("redo did not restore the edit")
	}
	demo.readOnly = true
	d.Frame()
	d.Type("blocked")
	if st.Snapshot().String() != edited {
		t.Fatal("read-only editor accepted typing")
	}
	demo.readOnly = false
	d.Frame()
	d.Key(input.KeyHome, 0)
	d.Type("ok")
	if st.Snapshot().String() == edited {
		t.Fatal("editable state was not restored")
	}
}

func TestCodeEditorDemoLanguagesHaveActualTokensAndKeepModels(t *testing.T) {
	demo := newCodeEditorDemo(false)
	defer demo.close()
	for file, want := range map[string]string{"main.go": "go", "app.ts": "typescript", "script.py": "python", "app.js": "javascript"} {
		demo.file = file
		d := ui.NewDriver(demo.render, ui.DriverOptions{Width: 1000, Height: 560})
		d.Frame()
		st := demo.activeState()
		if st.Model.Language() != want {
			t.Fatalf("%s language=%s", file, st.Model.Language())
		}
		hl := st.Highlighter()
		if hl == nil {
			t.Fatalf("%s has no highlighter", file)
		}
		deadline := time.Now().Add(5 * time.Second)
		for !hl.Highlight(st.Snapshot(), st.Snapshot().LineCount(), 10*time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s highlighting did not finish", file)
			}
		}
		tokens, final := hl.Tokens(st.Snapshot(), 0, nil)
		if !final || len(tokens) == 0 {
			t.Fatalf("%s has no final syntax tokens", file)
		}
		typed := false
		for _, token := range tokens {
			if token.Type != "" {
				typed = true
			}
		}
		if !typed {
			t.Fatalf("%s tokens have no syntax classifications", file)
		}
		d.Runner().Close()
	}
	demo.file = "main.go"
	st := demo.activeState()
	st.Insert("saved ")
	demo.file = "app.ts"
	demo.activeState()
	demo.file = "main.go"
	if !strings.Contains(demo.activeState().Snapshot().String(), "saved ") {
		t.Fatal("file switching discarded edits")
	}
}

func TestCodeEditorLargeDemoIsScrollable(t *testing.T) {
	demo := newCodeEditorDemo(true)
	defer demo.close()
	d := ui.NewDriver(demo.render, ui.DriverOptions{Width: 1000, Height: 560})
	defer d.Runner().Close()
	d.Frame()
	if demo.activeState().Snapshot().LineCount() < 2000 {
		t.Fatal("large sample missing")
	}
	before := demo.activeState().Viewport.TopLine
	d.Scroll(geom.Pt(400, 200), 800)
	if demo.activeState().Viewport.TopLine <= before {
		t.Fatal("code editor viewport did not scroll")
	}
}
