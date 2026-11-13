package gpuconf

import (
	"strings"
	"testing"
)

// TestSkipOption: Skip sees each subtest's path relative to the calling
// test, skips exactly the subtests it returns true for (without running
// them), and skipping a parent skips its children.
func TestSkipOption(t *testing.T) {
	var asked []string
	c := &suite{root: t.Name() + "/"}
	Skip(func(name string) bool {
		asked = append(asked, name)
		return strings.Contains(name, "images") || name == "Layers"
	})(c)
	Skip(func(name string) bool { return name == "Scenes/text@2" })(c)
	ran := map[string]bool{}
	mark := func(name string) func(*testing.T) { return func(*testing.T) { ran[name] = true } }
	c.run(t, "Scenes", func(t *testing.T) {
		ran["Scenes"] = true
		c.run(t, "solid@1", mark("solid@1"))
		c.run(t, "images@1", mark("images@1"))
		c.run(t, "text@2", mark("text@2"))
	})
	c.run(t, "Layers", func(t *testing.T) {
		ran["Layers"] = true
		c.run(t, "child", mark("Layers/child"))
	})
	for name, want := range map[string]bool{"Scenes": true, "solid@1": true, "images@1": false, "text@2": false, "Layers": false, "Layers/child": false} {
		if ran[name] != want {
			t.Errorf("%s ran = %v, want %v", name, ran[name], want)
		}
	}
	if got, want := strings.Join(asked, ","), "Scenes,Scenes/solid@1,Scenes/images@1,Scenes/text@2,Layers"; got != want {
		t.Errorf("Skip asked about %s, want %s", got, want)
	}
}
