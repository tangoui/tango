package text

import (
	"os"
	"path/filepath"
)

func systemFamilies() (sans, mono []*family) {
	dir := os.Getenv("WINDIR")
	if dir == "" {
		dir = `C:\Windows`
	}
	fd := filepath.Join(dir, "Fonts")
	p := func(n string) string { return filepath.Join(fd, n) }
	// Segoe UI ships separate files per weight; register each as its own
	// family entry under one name so weight matching works.
	segoe := &family{name: "Segoe UI", load: func() []*Face {
		var out []*Face
		for _, n := range []string{"segoeui.ttf", "seguisb.ttf", "segoeuib.ttf", "segoeuil.ttf",
			"segoeuii.ttf", "seguisbi.ttf", "segoeuiz.ttf", "seguili.ttf"} {
			f, err := os.Open(p(n))
			if err != nil {
				continue
			}
			faces, err := parseFaces("Segoe UI", f, "")
			if err != nil {
				f.Close()
				continue
			}
			out = append(out, faces...)
		}
		return out
	}}
	yahei := &family{name: "Microsoft YaHei", load: func() []*Face {
		var out []*Face
		for _, n := range []string{"msyh.ttc", "msyhbd.ttc", "msyh.ttf", "msyhbd.ttf"} {
			f, err := os.Open(p(n))
			if err != nil {
				continue
			}
			faces, err := parseFaces("Microsoft YaHei", f, "Microsoft YaHei\x00Microsoft YaHei UI\x00微软雅黑")
			if err != nil || len(faces) == 0 {
				f.Close()
				continue
			}
			out = append(out, faces...)
		}
		return out
	}}
	sans = []*family{segoe, yahei,
		sysFont{name: "SimSun", paths: []string{p("simsun.ttc")}}.family(),
		sysFont{name: "Arial", paths: []string{p("arial.ttf")}, extra: []string{p("ariali.ttf"), p("arialbi.ttf")}}.family(),
		sysFont{name: "Segoe UI Symbol", paths: []string{p("seguisym.ttf")}}.family(),
	}
	mono = []*family{sysFont{name: "Consolas", paths: []string{p("consola.ttf")}, extra: []string{p("consolai.ttf")}}.family()}
	return
}
