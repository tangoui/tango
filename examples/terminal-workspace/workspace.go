package main

import (
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/tangoui/tango/ui/terminal"
)

const maxTabs, maxPanes = 12, 4

type splitAxis uint8

const (
	splitRight splitAxis = iota
	splitDown
)

// Sessions belong to the application, not to a rendered view. Hiding a tab
// therefore never closes its PTY or replaces its emulator/scrollback.
type session struct {
	id, name  string
	pid       int
	term      *terminal.Terminal
	closeConn func() error
	once      sync.Once
}

func (s *session) close() {
	s.once.Do(func() {
		if s.closeConn != nil {
			if err := s.closeConn(); err != nil {
				log.Printf("terminal workspace: close %s: %v", s.id, err)
			}
		}
	})
}

type splitNode struct {
	id            string
	pane          *session
	axis          splitAxis
	first, second *splitNode
}

func (n *splitNode) panes() []*session {
	if n == nil {
		return nil
	}
	if n.pane != nil {
		return []*session{n.pane}
	}
	return append(n.first.panes(), n.second.panes()...)
}

func (n *splitNode) find(id string) *splitNode {
	if n == nil {
		return nil
	}
	if n.pane != nil {
		if n.pane.id == id {
			return n
		}
		return nil
	}
	if found := n.first.find(id); found != nil {
		return found
	}
	return n.second.find(id)
}

func removePane(n *splitNode, id string) *splitNode {
	if n == nil {
		return nil
	}
	if n.pane != nil {
		if n.pane.id == id {
			return nil
		}
		return n
	}
	n.first, n.second = removePane(n.first, id), removePane(n.second, id)
	if n.first == nil {
		return n.second
	}
	if n.second == nil {
		return n.first
	}
	return n
}

type terminalTab struct {
	id, name, activePaneID string
	root                   *splitNode
}

func (t *terminalTab) panes() []*session { return t.root.panes() }

type workspace struct {
	tabs        []*terminalTab
	activeTabID string
	next        int
	shellNumber int
	openSession func(id string) (*session, error)
}

func (w *workspace) nextID(prefix string) string {
	w.next++
	return fmt.Sprintf("%s-%d", prefix, w.next)
}

func (w *workspace) activeTab() *terminalTab {
	for _, tab := range w.tabs {
		if tab.id == w.activeTabID {
			return tab
		}
	}
	return nil
}

func (w *workspace) activeSession() *session {
	if tab := w.activeTab(); tab != nil {
		if n := tab.root.find(tab.activePaneID); n != nil {
			return n.pane
		}
	}
	return nil
}

func (w *workspace) open() (*session, error) {
	if w.openSession == nil {
		return nil, errors.New("no shell configured")
	}
	id := w.nextID("pane")
	s, err := w.openSession(id)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("shell factory returned no session")
	}
	s.id = id
	w.shellNumber++
	if s.name == "" {
		s.name = fmt.Sprintf("Shell %d", w.shellNumber)
	}
	return s, nil
}

func (w *workspace) newTab() error {
	if len(w.tabs) >= maxTabs {
		return fmt.Errorf("maximum %d tabs reached", maxTabs)
	}
	s, err := w.open()
	if err != nil {
		return err
	}
	tab := &terminalTab{id: w.nextID("tab"), name: s.name, activePaneID: s.id, root: &splitNode{id: s.id, pane: s}}
	w.tabs = append(w.tabs, tab)
	w.activeTabID = tab.id
	return nil
}

func (w *workspace) split(axis splitAxis) error {
	tab := w.activeTab()
	if tab == nil {
		return w.newTab()
	}
	if len(tab.panes()) >= maxPanes {
		return fmt.Errorf("maximum %d panes per tab reached", maxPanes)
	}
	n := tab.root.find(tab.activePaneID)
	if n == nil {
		return errors.New("no active pane to split")
	}
	s, err := w.open()
	if err != nil {
		return err
	}
	old := *n
	*n = splitNode{id: w.nextID("split"), axis: axis, first: &old, second: &splitNode{id: s.id, pane: s}}
	tab.activePaneID = s.id
	return nil
}

func (w *workspace) selectTab(id string) {
	for _, tab := range w.tabs {
		if tab.id == id {
			w.activeTabID = id
			return
		}
	}
}

func (w *workspace) cycleTab(delta int) {
	for i, tab := range w.tabs {
		if tab.id == w.activeTabID {
			n := len(w.tabs)
			w.activeTabID = w.tabs[((i+delta)%n+n)%n].id
			return
		}
	}
}

func (w *workspace) cyclePane() {
	if tab := w.activeTab(); tab != nil {
		panes := tab.panes()
		for i, pane := range panes {
			if pane.id == tab.activePaneID {
				tab.activePaneID = panes[(i+1)%len(panes)].id
				return
			}
		}
	}
}

func (w *workspace) closePane() {
	tab := w.activeTab()
	if tab == nil {
		return
	}
	panes := tab.panes()
	if len(panes) == 1 {
		w.closeTab(tab.id)
		return
	}
	for i, pane := range panes {
		if pane.id != tab.activePaneID {
			continue
		}
		tab.root = removePane(tab.root, pane.id)
		remaining := tab.panes()
		tab.activePaneID = remaining[min(i, len(remaining)-1)].id
		pane.close()
		return
	}
}

func (w *workspace) closeTab(id string) {
	for i, tab := range w.tabs {
		if tab.id != id {
			continue
		}
		w.tabs = append(w.tabs[:i:i], w.tabs[i+1:]...)
		if w.activeTabID == id {
			w.activeTabID = ""
			if len(w.tabs) > 0 {
				w.activeTabID = w.tabs[min(i, len(w.tabs)-1)].id
			}
		}
		for _, pane := range tab.panes() {
			pane.close()
		}
		return
	}
}

func (w *workspace) close() {
	for len(w.tabs) > 0 {
		w.closeTab(w.tabs[0].id)
	}
}
