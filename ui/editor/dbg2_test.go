package editor
import ("testing";"github.com/tangoui/tango/ui/editor/regex";"github.com/tangoui/tango/ui/editor/langs")
func TestDbgTr(t *testing.T){
	c := langs.ByID("typescript").Conf
	g, ok := regex.Translate(c.Folding.Start.Source, "")
	t.Logf("%q %v", g, ok)
	m := markersOf(&langs.ByID("typescript").Conf)
	t.Logf("%+v", m)
}
