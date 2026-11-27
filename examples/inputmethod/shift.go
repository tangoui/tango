package main

import (
	"log"
	"time"

	"github.com/tangoui/tango/sys/ime"
)

// shiftTap tracks a single, unchorded Shift press in the active session.
type shiftTap struct {
	held  map[string]bool
	code  string
	since time.Time
}

func (t *shiftTap) Down(code string, now time.Time) {
	if len(t.held) == 0 && (code == "ShiftLeft" || code == "ShiftRight") {
		t.code, t.since = code, now
	} else {
		t.Key()
	}
	if code == "CapsLock" {
		return
	}
	if t.held == nil {
		t.held = make(map[string]bool)
	}
	t.held[code] = true
}

func (t *shiftTap) Key() { t.code = "" }

func (t *shiftTap) Up(code string, now time.Time) bool {
	delete(t.held, code)
	elapsed := now.Sub(t.since)
	match := t.code != "" && t.code == code && elapsed >= 0 && elapsed <= 500*time.Millisecond
	t.Key()
	return match
}

type pinyinMode struct {
	current string
	set     func(string) error
	flush   func()
	about   func()
}

func (m *pinyinMode) toggle() {
	next := "en"
	if m.current == "en" {
		next = "zh"
	}
	m.flush()
	if err := m.set(next); err != nil {
		log.Printf("Tango Pinyin mode: %v", err)
		return
	}
	m.current = next
}

func (m *pinyinMode) changed(id string) {
	if m.current != id {
		m.current = id
		m.flush()
	}
}

func (m *pinyinMode) menu() []ime.MenuItem {
	return []ime.MenuItem{
		{ID: "toggle", Title: "切换中/英", Checked: m.current == "zh"},
		{Separator: true},
		{ID: "about", Title: "关于"},
	}
}

func (m *pinyinMode) click(id string) {
	switch id {
	case "toggle":
		m.toggle()
	case "about":
		m.about()
	}
}
