package diffview

import (
	"strings"
	"sync"
	"testing"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/theme"
)

// Diff views of windows on different UI threads open, run commands and
// close at the same time: the registry the commands look views up in must
// be safe for that (run with -race).
func TestDiffViewsOnTwoThreads(t *testing.T) {
	var wg sync.WaitGroup
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				om := editor.NewModel("a.go", strings.Join([]string{"a", "b", "c"}, "\n"), editor.DefaultRegistry())
				mm := editor.NewModel("b.go", strings.Join([]string{"a", "x", "c"}, "\n"), editor.DefaultRegistry())
				d := New(om, mm, theme.Base("vs"))
				d.Modified.Execute(CmdNext, nil)
				d.Close()
			}
		}()
	}
	wg.Wait()
}
