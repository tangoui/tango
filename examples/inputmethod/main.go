// The Tango input method example hosts a small Pinyin engine.
package main

import (
	"errors"
	"log"
	"strings"
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
	var candidates *candidateWindow
	flush := func(s *ime.Session) {
		candidates.hide(s)
		if c := composers[s.ID()]; c != nil {
			delete(composers, s.ID())
			if text := c.Flush(); text != "" {
				s.Commit(text)
			}
		}
	}
	app.OnReady(func() {
		var err error
		candidates, err = newCandidateWindow(app)
		if err != nil && !errors.Is(err, tango.ErrUnsupported) {
			log.Printf("Tango Pinyin candidates: %v", err)
		}
		err = ime.Serve(app, ime.Handler{
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
				if len(res.Candidates) == 0 && (res.Handled || raw != "") {
					candidates.hide(s)
				}
				if raw != "" {
					s.Commit(raw)
				}
				if res.Commit != "" {
					s.Commit(res.Commit)
				} else if res.Handled {
					s.SetComposition(res.Preedit, utf8.RuneCountInString(res.Preedit))
				}
				if candidates != nil && len(res.Candidates) != 0 {
					candidates.show(s, s.Caret(), res.Candidates, func(index int) {
						if text, ok := c.Pick(index); ok {
							delete(composers, s.ID())
							candidates.hide(s)
							s.Commit(text)
						}
					})
				}
				return consumed
			},
			Deactivate:        flush,
			CommitComposition: flush,
			End: func(s *ime.Session) {
				endSession(s, composers, candidates)
			},
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

func endSession(s *ime.Session, composers map[uint64]*engine.Composer, candidates *candidateWindow) {
	delete(composers, s.ID())
	candidates.hide(s)
}

func handle(c *engine.Composer, k ime.Key) (res engine.Result, flush string, consumed bool) {
	if k.Ctrl || k.Alt || k.Meta {
		return engine.Result{}, c.Flush(), false
	}
	digit, ok := strings.CutPrefix(k.Code, "Digit")
	if !ok {
		digit, ok = strings.CutPrefix(k.Code, "Numpad")
	}
	if !k.Shift && ok &&
		len(digit) == 1 && digit[0] >= '1' && digit[0] <= '9' {
		if text, ok := c.Pick(int(digit[0] - '1')); ok {
			return engine.Result{Handled: true, Commit: text}, "", true
		}
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
