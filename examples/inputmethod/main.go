// The Tango input method example is launched by macOS after installation.
package main

import (
	"log"
	"unicode/utf8"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/examples/inputmethod/engine"
	"github.com/tangoui/tango/sys/ime"
)

func main() {
	quitOnLastClose := false
	app := tango.New(tango.Options{
		Name: "Tango Pinyin", ID: "com.tangoui.inputmethod.example",
		QuitOnLastClose: &quitOnLastClose,
	})
	composers := make(map[uint64]*engine.Composer)
	flush := func(s *ime.Session) {
		if c := composers[s.ID()]; c != nil {
			delete(composers, s.ID())
			if text := c.Flush(); text != "" {
				s.Commit(text)
			}
		}
	}
	app.OnReady(func() {
		err := ime.Serve(app, ime.Handler{
			Key: func(s *ime.Session, k ime.Key) bool {
				c := composers[s.ID()]
				if c == nil {
					c = engine.New()
				}
				res, raw, consumed := handle(c, k)
				if c.Composing() {
					composers[s.ID()] = c
				} else {
					delete(composers, s.ID())
				}
				if raw != "" {
					s.Commit(raw)
				}
				if res.Commit != "" {
					s.Commit(res.Commit)
				} else if res.Handled {
					s.SetComposition(res.Preedit, utf8.RuneCountInString(res.Preedit))
				}
				return consumed
			},
			Deactivate:        flush,
			CommitComposition: flush,
		})
		if err != nil {
			log.Printf("Tango Pinyin: %v", err)
			app.Quit()
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func handle(c *engine.Composer, k ime.Key) (res engine.Result, flush string, consumed bool) {
	if k.Ctrl || k.Alt || k.Meta {
		return engine.Result{}, c.Flush(), false
	}
	var key engine.Key
	switch k.Code {
	case "Space":
		key.Special = engine.Space
	case "Enter", "NumpadEnter":
		key.Special = engine.Enter
	case "Backspace":
		key.Special = engine.Backspace
	case "Escape":
		key.Special = engine.Escape
	case "Tab", "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight":
		return engine.Result{}, "", c.Composing()
	default:
		if k.Text == "" {
			return engine.Result{}, "", false
		}
		key.Rune, _ = utf8.DecodeRuneInString(k.Text)
	}
	res = c.Feed(key)
	return res, "", res.Handled
}
