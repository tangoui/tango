// The Tango input method example hosts a small Pinyin engine.
package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/examples/inputmethod/engine"
	"github.com/tangoui/tango/sys/ime"
)

func main() {
	if runtime.GOOS == "windows" {
		if cache, err := os.UserCacheDir(); err == nil {
			dir := filepath.Join(cache, "Tango Pinyin")
			if err := os.MkdirAll(dir, 0o755); err == nil {
				if file, err := os.OpenFile(filepath.Join(dir, "ime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
					log.SetOutput(file)
					defer file.Close()
				}
			}
		}
	}
	quitOnLastClose := false
	app := tango.New(tango.Options{
		Name: "Tango Pinyin", ID: "com.tangoui.inputmethod.example",
		QuitOnLastClose: &quitOnLastClose,
	})
	composers := make(map[uint64]*engine.Composer)
	var candidates *candidateWindow
	var active *ime.Session
	var tap shiftTap
	systemCandidates := ime.SystemDrawsCandidates()
	hideCandidates := func(s *ime.Session) {
		if systemCandidates {
			s.SetCandidates(nil, -1)
		} else {
			candidates.hide(s)
		}
	}
	flush := func(s *ime.Session) {
		hideCandidates(s)
		if c := composers[s.ID()]; c != nil {
			delete(composers, s.ID())
			if text := c.Flush(); text != "" {
				s.Commit(text)
			}
		}
	}
	mode := pinyinMode{
		current: "zh",
		set:     func(id string) error { return ime.SetMode(app, id) },
		flush: func() {
			if active != nil {
				flush(active)
			}
		},
		about: func() { log.Print("关于 Tango Pinyin") },
	}
	app.OnReady(func() {
		if !systemCandidates {
			var err error
			candidates, err = newCandidateWindow(app)
			if err != nil && !errors.Is(err, tango.ErrUnsupported) {
				log.Printf("Tango Pinyin candidates: %v", err)
			}
		}
		err := ime.Serve(app, ime.Handler{
			Key: func(s *ime.Session, k ime.Key) bool {
				tap.Key()
				c := composers[s.ID()]
				if c == nil {
					c = engine.New()
				}
				res, raw, consumed := handle(c, k, mode.current)
				if c.Composing() {
					composers[s.ID()] = c
				} else {
					delete(composers, s.ID())
				}
				if len(res.Candidates) == 0 && (res.Handled || raw != "") {
					hideCandidates(s)
				}
				if raw != "" {
					s.Commit(raw)
				}
				if res.Commit != "" {
					s.Commit(res.Commit)
				} else if res.Handled {
					s.SetComposition(res.Preedit, utf8.RuneCountInString(res.Preedit))
				}
				if len(res.Candidates) != 0 {
					if systemCandidates {
						list := make([]ime.Candidate, len(res.Candidates))
						for i, text := range res.Candidates {
							list[i] = ime.Candidate{Text: text}
						}
						s.SetCandidates(list, 0)
					} else if candidates != nil {
						candidates.show(s, s.Caret(), res.Candidates, func(index int) {
							if text, ok := c.Pick(index); ok {
								delete(composers, s.ID())
								hideCandidates(s)
								s.Commit(text)
							}
						})
					}
				}
				return consumed
			},
			Activate: func(s *ime.Session) {
				active, tap = s, shiftTap{}
			},
			Deactivate: func(s *ime.Session) {
				flush(s)
				if active == s {
					active, tap = nil, shiftTap{}
				}
			},
			CommitComposition: flush,
			End: func(s *ime.Session) {
				endSession(s, composers, candidates)
				if active == s {
					active, tap = nil, shiftTap{}
				}
			},
			Modifier: func(s *ime.Session, m ime.ModifierKey) bool {
				if active != s {
					return false
				}
				if tap.modifier(m, time.Now()) {
					mode.toggle()
				}
				return false
			},
			ModeChanged: func(id string) {
				mode.changed(id)
				log.Printf("Tango Pinyin system mode: %s", id)
			},
			Menu:        mode.menu,
			MenuClicked: mode.click,
		})
		if err != nil {
			log.Printf("Tango Pinyin: %v", err)
			app.Quit()
			return
		}
		if current := ime.CurrentMode(app); current != "" {
			mode.changed(current)
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

func handle(c *engine.Composer, k ime.Key, mode string) (res engine.Result, flush string, consumed bool) {
	if mode == "en" || k.Ctrl || k.Alt || k.Meta ||
		(k.CapsLock || k.Shift) && strings.HasPrefix(k.Code, "Key") {
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
