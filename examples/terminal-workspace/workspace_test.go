package main

import (
	"errors"
	"testing"
)

func testWorkspace() (*workspace, map[string]int) {
	closed := map[string]int{}
	w := &workspace{openSession: func(id string) (*session, error) {
		return &session{id: id, closeConn: func() error { closed[id]++; return nil }}, nil
	}}
	return w, closed
}

func TestWorkspaceTabSessionsSurviveSwitching(t *testing.T) {
	w, closed := testWorkspace()
	defer w.close()
	if err := w.newTab(); err != nil {
		t.Fatal(err)
	}
	firstTab, first := w.activeTab(), w.activeSession()
	if err := w.newTab(); err != nil {
		t.Fatal(err)
	}
	secondTab, second := w.activeTab(), w.activeSession()
	if first == second || first.id == second.id {
		t.Fatal("tabs share a session")
	}
	for range 3 {
		w.cycleTab(1)
		if w.activeSession() != first {
			t.Fatal("first session changed")
		}
		w.cycleTab(-1)
		if w.activeSession() != second {
			t.Fatal("second session changed")
		}
	}
	w.closeTab(firstTab.id)
	if w.activeTab() != secondTab || w.activeSession() != second {
		t.Fatal("closing inactive tab changed selection")
	}
	if closed[first.id] != 1 || closed[second.id] != 0 {
		t.Fatalf("closed wrong sessions: %v", closed)
	}
}

func TestWorkspaceNestedSplitCollapse(t *testing.T) {
	w, closed := testWorkspace()
	defer w.close()
	_ = w.newTab()
	first := w.activeSession()
	if err := w.split(splitRight); err != nil {
		t.Fatal(err)
	}
	second := w.activeSession()
	if err := w.split(splitDown); err != nil {
		t.Fatal(err)
	}
	third := w.activeSession()
	if len(w.activeTab().panes()) != 3 {
		t.Fatal("missing panes")
	}
	w.closePane()
	if w.activeSession() != second {
		t.Fatal("closing nested split did not select survivor")
	}
	if w.activeTab().root.second.pane != second {
		t.Fatal("nested split did not collapse")
	}
	if closed[third.id] != 1 || closed[first.id] != 0 {
		t.Fatalf("closed wrong pane: %v", closed)
	}
	w.cyclePane()
	if w.activeSession() != first {
		t.Fatal("pane focus did not wrap")
	}
	w.closePane()
	if w.activeTab().root.pane != second {
		t.Fatal("outer split did not collapse")
	}
}

func TestWorkspaceLaunchFailureRollsBack(t *testing.T) {
	w, _ := testWorkspace()
	defer w.close()
	_ = w.newTab()
	tab, root, pane := w.activeTab(), w.activeTab().root, w.activeSession()
	w.openSession = func(string) (*session, error) { return nil, errors.New("shell unavailable") }
	if err := w.split(splitRight); err == nil {
		t.Fatal("failed split was accepted")
	}
	if err := w.newTab(); err == nil {
		t.Fatal("failed tab was accepted")
	}
	if w.activeTab() != tab || tab.root != root || w.activeSession() != pane || len(w.tabs) != 1 {
		t.Fatal("failed launch changed the live layout")
	}
}

func TestWorkspaceLimitsAndCleanup(t *testing.T) {
	w, closed := testWorkspace()
	_ = w.newTab()
	for range 3 {
		if err := w.split(splitRight); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.split(splitRight); err == nil {
		t.Fatal("more than four panes allowed")
	}
	for range 11 {
		if err := w.newTab(); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.newTab(); err == nil {
		t.Fatal("more than twelve tabs allowed")
	}
	w.close()
	w.close()
	if len(closed) != 15 {
		t.Fatalf("closed %d sessions, want 15", len(closed))
	}
	for id, count := range closed {
		if count != 1 {
			t.Fatalf("session %s closed %d times", id, count)
		}
	}
	if len(w.tabs) != 0 || w.activeSession() != nil {
		t.Fatal("workspace retained closed sessions")
	}
}

func TestWorkspaceCloseLastAndReopen(t *testing.T) {
	w, closed := testWorkspace()
	_ = w.newTab()
	first := w.activeSession()
	w.selectTab("missing")
	if w.activeSession() != first {
		t.Fatal("unknown tab changed selection")
	}
	w.closePane()
	w.cycleTab(1)
	w.cyclePane()
	w.closePane()
	if len(w.tabs) != 0 || w.activeSession() != nil || closed[first.id] != 1 {
		t.Fatal("last pane not cleaned up")
	}
	if err := w.newTab(); err != nil {
		t.Fatal(err)
	}
	if w.activeSession().id == first.id {
		t.Fatal("reused closed session identity")
	}
	w.close()
}
