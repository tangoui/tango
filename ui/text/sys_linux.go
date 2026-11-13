//go:build linux && !android

package text

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func systemFamilies() (sans, mono []*family) {
	home, _ := os.UserHomeDir()
	dirs := []string{"/usr/share/fonts", "/usr/local/share/fonts", "/system/fonts"}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".local/share/fonts"), filepath.Join(home, ".fonts"))
	}
	index := map[string]string{}
	for _, d := range dirs {
		filepath.WalkDir(d, func(path string, de fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if !de.IsDir() {
				n := strings.ToLower(de.Name())
				if _, ok := index[n]; !ok {
					index[n] = path
				}
			}
			return nil
		})
	}
	var multiKeep func(name, keep string, files ...string) *family
	find := func(names ...string) []string {
		var out []string
		for _, n := range names {
			if p, ok := index[strings.ToLower(n)]; ok {
				out = append(out, p)
			}
		}
		return out
	}
	multi := func(name string, files ...string) *family { return multiKeep(name, "", files...) }
	multiKeep = func(name, keep string, files ...string) *family {
		return &family{name: name, load: func() []*Face {
			var out []*Face
			for _, p := range find(files...) {
				f, err := os.Open(p)
				if err != nil {
					continue
				}
				faces, err := parseFaces(name, f, keep)
				if err != nil || len(faces) == 0 {
					f.Close()
					continue
				}
				out = append(out, faces...)
			}
			return out
		}}
	}
	sans = []*family{
		multi("Noto Sans", "NotoSans-Regular.ttf", "NotoSans-Medium.ttf", "NotoSans-SemiBold.ttf", "NotoSans-Bold.ttf", "NotoSans[wdth,wght].ttf",
			"NotoSans-Italic.ttf", "NotoSans-MediumItalic.ttf", "NotoSans-SemiBoldItalic.ttf", "NotoSans-BoldItalic.ttf", "NotoSans-Italic[wdth,wght].ttf"),
		multi("DejaVu Sans", "DejaVuSans.ttf", "DejaVuSans-Bold.ttf", "DejaVuSans-Oblique.ttf", "DejaVuSans-BoldOblique.ttf"),
		multi("Roboto", "Roboto-Regular.ttf", "Roboto-Medium.ttf", "Roboto-Bold.ttf", "Roboto-Italic.ttf", "Roboto-MediumItalic.ttf", "Roboto-BoldItalic.ttf"),
		multiKeep("Noto Sans CJK SC", "Noto Sans CJK SC\x00Noto Sans SC", "NotoSansCJK-Regular.ttc", "NotoSansCJK-Bold.ttc", "NotoSansCJKsc-Regular.otf", "NotoSansCJKsc-Bold.otf", "NotoSansSC-Regular.otf", "NotoSansSC-Bold.otf", "NotoSansSC-Regular.ttf", "NotoSansCJK-VF.ttc"),
		sysFont{name: "WenQuanYi", paths: find("wqy-microhei.ttc", "wqy-zenhei.ttc")}.family(),
		sysFont{name: "Droid Sans Fallback", paths: find("DroidSansFallbackFull.ttf", "DroidSansFallback.ttf")}.family(),
	}
	mono = []*family{multi("DejaVu Sans Mono", "DejaVuSansMono.ttf", "NotoSansMono-Regular.ttf", "DejaVuSansMono-Oblique.ttf")}
	return
}
