package editor
import ("testing";"time";"github.com/tangoui/tango/ui/editor/langs")
func TestDbgSpeed(t *testing.T){
	for _, f := range []string{"a.go","a.ts"} {
		st := NewState(longGoDoc(290000), f, nil)
		conf := st.Model.Conf()
		t0 := time.Now()
		rs := indentRanges(st.Snapshot(), 4, false, markersOf(conf), 5000)
		t.Log(f, len(rs), time.Since(t0), langs.ByID("go") != nil)
	}
}
