package recent

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
)

func setup(t *testing.T) string {
	dir := t.TempDir()
	old := configDir
	configDir = func(id string) (string, error) { return filepath.Join(dir, id), nil }
	t.Cleanup(func() { configDir = old })
	return dir
}

func touch(t *testing.T, dir, name string) string {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAddListClear(t *testing.T) {
	dir := setup(t)
	a, fb := faketest.Run(t, tango.Options{ID: "com.example.recent"})
	var files []string
	for i := range Max + 2 {
		files = append(files, touch(t, dir, "f"+strconv.Itoa(i)))
	}
	for _, f := range files {
		if err := Add(a, f); err != nil {
			t.Fatal(err)
		}
	}
	if err := Add(a, files[5]); err != nil { // repeat moves up
		t.Fatal(err)
	}
	got, err := List(a)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != Max || got[0] != files[5] || got[1] != files[Max+1] || slices.Contains(got, files[0]) {
		t.Fatalf("list %v", got)
	}
	if slices.Index(got[1:], files[5]) >= 0 {
		t.Fatal("duplicate entry")
	}

	// Missing files are left out.
	os.Remove(files[Max+1])
	got, _ = List(a)
	if slices.Contains(got, files[Max+1]) || len(got) != Max-1 {
		t.Fatalf("list after delete %v", got)
	}
	if err := Remove(a, files[5]); err != nil {
		t.Fatal(err)
	}
	if got, _ = List(a); slices.Contains(got, files[5]) {
		t.Fatalf("list after Remove %v", got)
	}

	// The OS list (fake: NoteRecentDocument) follows Add and Clear.
	waitFor(t, func() bool { return len(fb.RecentDocuments()) == Max+3 })
	if err := Clear(a); err != nil {
		t.Fatal(err)
	}
	if got, _ = List(a); len(got) != 0 {
		t.Fatalf("list after Clear %v", got)
	}
	waitFor(t, func() bool { return len(fb.RecentDocuments()) == 0 })
}

func TestCorruptFile(t *testing.T) {
	dir := setup(t)
	a, _ := faketest.NewMinimalApp(tango.Options{Name: "Notes"})
	if err := os.MkdirAll(filepath.Join(dir, "Notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "Notes", "recent.json"), []byte("{"), 0o600)
	f := touch(t, dir, "x")
	if err := Add(a, f); err != nil {
		t.Fatal(err)
	}
	if got, _ := List(a); !slices.Equal(got, []string{f}) {
		t.Fatalf("got %v", got)
	}
}

func TestNoID(t *testing.T) {
	setup(t)
	a, _ := faketest.NewApp(tango.Options{})
	if err := Add(a, "/x"); err == nil {
		t.Fatal("Add without ID succeeded")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
