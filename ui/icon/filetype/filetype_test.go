package filetype

import (
	"image"
	"testing"

	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/icon/antd"
	"github.com/tangoui/tango/ui/icon/lucide"
	"github.com/tangoui/tango/ui/theme"
)

func TestKindOf(t *testing.T) {
	for name, want := range map[string]Kind{
		"main.go": Code, "src/App.vue": Vue, `C:\x\index.TSX`: TypeScript, "Dockerfile": Terminal,
		"Dockerfile.dev": Terminal, ".env.local": Config, "package.json": Archive, "README.md": Markdown,
		"photo.JPEG": Image, "a.tar.gz": Archive, "notes": Document, "LICENSE": Text, "report.pdf": PDF,
		"": Document, "data.yaml": Data, "style.scss": Style,
	} {
		if got := KindOf(name, false); got != want {
			t.Errorf("KindOf(%q) = %v, want %v", name, got, want)
		}
	}
	if KindOf("src", true) != Folder {
		t.Error("folder")
	}
}

func TestSets(t *testing.T) {
	for _, s := range []*Set{Ant, Element, Lucide} {
		for k := Kind(0); k < numKinds; k++ {
			if s.Kinds[k] == nil {
				t.Errorf("%v: no icon", k)
			}
		}
	}
	if Lucide.Icon("src", true, true) != lucide.FolderOpen || Lucide.Icon("src", true, false) != lucide.Folder {
		t.Error("folder open/closed")
	}
	if ForFlavor(theme.StyleShadcn) != Lucide || ForFlavor(theme.Element) != Element || ForFlavor(theme.Ant) != Ant {
		t.Error("ForFlavor")
	}
}

func TestTheme(t *testing.T) {
	ts := icon.FromImage("plugin:ts", image.NewAlpha(image.Rect(0, 0, 16, 16)), true)
	dts := icon.New("plugin:dts", "M4 4H20", "")
	git := icon.New("plugin:git", "M4 4H20", "")
	th := &Theme{
		Extensions:  map[string]*icon.Icon{"ts": ts, "d.ts": dts},
		FileNames:   map[string]*icon.Icon{".gitignore": git},
		FolderNames: map[string]*icon.Icon{".git": git},
	}
	for name, want := range map[string]*icon.Icon{
		"a.ts": ts, "types/x.d.ts": dts, ".gitignore": git, "main.go": antd.CodeOutlined, "x": antd.FileOutlined,
	} {
		if got := th.Icon(name, false, false); got != want {
			t.Errorf("%s: %v, want %v", name, got.Name, want.Name)
		}
	}
	if th.Icon(".git", true, true) != git || th.Icon("src", true, true) != antd.FolderOpenOutlined {
		t.Error("folders")
	}
	if !ts.IsImage() {
		t.Error("FromImage")
	}
}
