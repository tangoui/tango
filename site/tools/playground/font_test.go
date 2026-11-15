package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"

	"github.com/go-text/typesetting/font"
)

func TestBundledFontContainsChineseGalleryLabels(t *testing.T) {
	f, err := font.ParseTTF(bytes.NewReader(demoChineseFont))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range "按钮输入表格移动图选择文件只读行号撤销重做恢复示例代码高亮" {
		if _, ok := f.NominalGlyph(r); !ok {
			t.Errorf("missing glyph %c", r)
		}
	}
}

// Every Han character in the gallery and ui sources must be in the subset;
// otherwise the online demos drop it. Regenerate with
// site/tools/subset-gallery-font.py (see fonts/README.md) when this fails.
func TestBundledFontCoversGallerySources(t *testing.T) {
	f, err := font.ParseTTF(bytes.NewReader(demoChineseFont))
	if err != nil {
		t.Fatal(err)
	}
	missing := map[rune]bool{}
	for _, dir := range []string{"../../../ui", "../../../examples/ui-gallery"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, r := range string(b) {
				if unicode.Is(unicode.Han, r) {
					if _, ok := f.NominalGlyph(r); !ok {
						missing[r] = true
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(missing) > 0 {
		rs := make([]rune, 0, len(missing))
		for r := range missing {
			rs = append(rs, r)
		}
		sort.Slice(rs, func(i, j int) bool { return rs[i] < rs[j] })
		t.Errorf("font subset lacks %d Han characters used in sources: %s", len(rs), string(rs))
	}
}
