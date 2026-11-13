package main

import (
	"bufio"
	"bytes"
	"cmp"
	"embed"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

// templates holds the project templates: go/ is shared by all of them, the
// other directories hold each template's front end and, where it differs
// from go/, its own Go files (rendered after go/, so they replace them).
// Files ending in .tmpl are rendered with [[ ]] delimiters (so Vue's {{ }}
// survives) and lose the suffix.
//
//go:embed all:templates
var templates embed.FS

// frontendTemplate describes a project template in templates/<name>.
type frontendTemplate struct {
	name     string
	summary  string
	frontend Frontend // zero for templates without a front end (self-drawn UI)
}

var frontendTemplates = []frontendTemplate{
	{name: "vanilla", summary: "plain HTML/CSS/JS, no npm needed", frontend: Frontend{Dir: "frontend", Dist: "."}},
	{name: "vanilla-ts", summary: "TypeScript + Vite", frontend: viteFrontend},
	{name: "react-ts", summary: "React + TypeScript + Vite", frontend: viteFrontend},
	{name: "vue-ts", summary: "Vue + TypeScript + Vite", frontend: viteFrontend},
	{name: "shadcn", summary: "React + TypeScript + Vite with Tailwind CSS and shadcn/ui components", frontend: viteFrontend},
	{name: "ui", summary: "self-drawn UI in Go (package ui): no WebView, no npm"},
	{name: "hybrid", summary: "WebView page (TypeScript + Vite) with a self-drawn tool panel on top", frontend: viteFrontend},
}

// defaultTemplate is used when -template is not given.
const defaultTemplate = "vanilla"

// defaultInitDir is the directory "tan init -yes" creates when none is given.
const defaultInitDir = "tango-app"

var viteFrontend = Frontend{Dir: "frontend", Dist: "dist", Dev: "npm run dev", Build: "npm run build"}

func findTemplate(name string) (frontendTemplate, bool) {
	i := slices.IndexFunc(frontendTemplates, func(t frontendTemplate) bool { return t.name == name })
	if i < 0 {
		return frontendTemplate{}, false
	}
	return frontendTemplates[i], true
}

// initOptions are the inputs of a new project.
type initOptions struct {
	Dir      string // target directory; must not exist or be empty
	Name     string // display name
	ID       string
	Module   string
	Template string
	Local    string // a Tango checkout to use through replace ("" for the released module)
	Tidy     bool   // run go mod tidy afterwards
	Mobile   bool   // write the "ios" and "android" sections with safe defaults
}

// stdinIsTerminal reports whether init may ask questions (a variable for
// tests).
var stdinIsTerminal = func() bool { return isTerminal(os.Stdin.Fd()) }

func runInit(args []string) error {
	fs := newFlags("init", "[flags] [dir]")
	tmpl := fs.String("template", defaultTemplate, "project `template`: "+templateNames())
	module := fs.String("module", "", "Go module `path` (default: the directory name)")
	name := fs.String("name", "", "app display `name` (default: the directory name)")
	id := fs.String("id", "", "app `id`, reverse-DNS (default: com.example.<name>)")
	local := fs.String("local", "", "use the Tango checkout in `dir` through a replace directive")
	mobile := fs.Bool("mobile", true, `write the "ios" and "android" sections of tan.json (safe defaults, no signing)`)
	yes := fs.Bool("yes", false, "do not ask questions; use the flags and defaults (directory "+defaultInitDir+" when none is given)")
	fs.Usage = func() { initUsage(fs) }
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return errors.New("init takes at most one directory")
	}
	o := initOptions{Name: *name, ID: *id, Module: *module, Template: *tmpl, Local: *local, Tidy: true, Mobile: *mobile}
	switch {
	case fs.NArg() == 1:
		o.Dir = fs.Arg(0)
	case *yes:
		o.Dir = defaultInitDir
	case stdinIsTerminal():
		if err := askInit(os.Stdin, os.Stderr, &o); err != nil {
			return err
		}
	default:
		fs.Usage()
		return errors.New("init needs a directory: pass one, run tan init in a terminal to answer a few questions, or add -yes to use the defaults")
	}
	if err := createProject(o); err != nil {
		return err
	}
	logf("Created %s. Next:", o.Dir)
	cd := o.Dir
	if strings.ContainsAny(cd, " '\"") {
		cd = strconv.Quote(cd)
	}
	fmt.Fprintf(os.Stderr, "    cd %s\n    tan dev\n", cd)
	return nil
}

// initUsage is "tan init -h".
func initUsage(fs *flag.FlagSet) {
	w := fs.Output()
	fmt.Fprint(w, `Usage: tan init [flags] [dir]

Creates a Tango project in dir (which must not exist or be empty). Without
dir, in a terminal, tan init asks for the directory, app name, id, template
and mobile support; -yes skips the questions.

Templates:
`)
	for _, t := range frontendTemplates {
		fmt.Fprintf(w, "  %-11s %s\n", t.name, t.summary)
	}
	fmt.Fprint(w, `
tan.json gets "permissions": [] and, unless -mobile=false, "ios" and
"android" sections with only minimum OS versions; signing settings are left
for you to add (docs/mobile.md).

Flags:
`)
	fs.PrintDefaults()
}

func templateNames() string {
	var names []string
	for _, t := range frontendTemplates {
		names = append(names, t.name)
	}
	return strings.Join(names, ", ")
}

// templateData is what .tmpl files see.
type templateData struct {
	Name, ID, Slug, Module string
	Embed                  string // go:embed path of the built front end
	Dist                   string // build output relative to the project; "" when the sources are served as is
	Runtime                string // package.json version spec of @tangoui/runtime
}

func createProject(o initOptions) error {
	ft, ok := findTemplate(o.Template)
	if !ok {
		return fmt.Errorf("unknown template %q (have %s)", o.Template, templateNames())
	}
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s exists and is not empty", dir)
	}
	base := filepath.Base(dir)
	if o.Name == "" {
		o.Name = base
	}
	s := slug(o.Name)
	if s == "" {
		return fmt.Errorf("cannot derive a project name from %q; pass -name", o.Name)
	}
	if o.ID == "" {
		o.ID = "com.example." + strings.ReplaceAll(s, "-", "")
	}
	if o.Module == "" {
		o.Module = s
	}
	cfg := &Config{
		Name:     o.Name,
		ID:       o.ID,
		Version:  "0.1.0",
		Icon:     "icon.png",
		Frontend: ft.frontend,
	}
	if o.Mobile {
		// Only minimum versions: permissions start empty and signing is
		// configured by the developer.
		cfg.IOS = IOS{MinVersion: defaultIOSMin}
		cfg.Android = Android{MinSdk: androidHostMinSdk}
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	data := templateData{
		Name:    o.Name,
		ID:      o.ID,
		Slug:    s,
		Module:  o.Module,
		Embed:   path.Clean(path.Join(ft.frontend.Dir, ft.frontend.Dist)),
		Dist:    builtDist(ft.frontend),
		Runtime: "^0.1.0",
	}
	var local string
	if o.Local != "" {
		if local, err = filepath.Abs(o.Local); err != nil {
			return err
		}
		if _, err := os.Stat(filepath.Join(local, "go.mod")); err != nil {
			return fmt.Errorf("-local %s: not a Tango checkout: %w", o.Local, err)
		}
		data.Runtime = "file:" + filepath.ToSlash(filepath.Join(local, "runtime-js"))
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := renderTree("templates/go", dir, data); err != nil {
		return err
	}
	if err := renderTree("templates/"+ft.name, dir, data); err != nil {
		return err
	}
	files := map[string][]byte{
		ConfigFile: projectConfig(cfg),
		"go.mod":   goMod(o.Module, local),
		"icon.png": defaultIcon(o.Name),
	}
	for name, b := range files {
		if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
			return err
		}
	}
	if o.Tidy {
		cmd := exec.Command("go", "mod", "tidy")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			warnf("go mod tidy failed; run it yourself once github.com/tangoui/tango is reachable:\n%s", out)
		}
	}
	return nil
}

// askInit asks for the project settings on in, writing the questions to
// out. The values already in o (from flags) are the defaults; Enter takes
// the default shown in brackets. Bad answers are asked again; the end of
// the input aborts.
func askInit(in io.Reader, out io.Writer, o *initOptions) error {
	q := &questioner{in: bufio.NewReader(in), out: out}
	fmt.Fprintln(out, "Create a Tango project (Enter takes the [default]).")

	dir, err := q.ask("Project directory", cmp.Or(o.Dir, defaultInitDir), func(v string) error {
		if entries, err := os.ReadDir(v); err == nil && len(entries) > 0 {
			return fmt.Errorf("%s exists and is not empty", v)
		}
		return nil
	})
	if err != nil {
		return err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	name, err := q.ask("App name", cmp.Or(o.Name, filepath.Base(abs)), func(v string) error {
		if slug(v) == "" {
			return errors.New("the name needs at least one letter or digit")
		}
		return nil
	})
	if err != nil {
		return err
	}
	id, err := q.ask("App id (reverse-DNS)", cmp.Or(o.ID, "com.example."+strings.ReplaceAll(slug(name), "-", "")), func(v string) error {
		if !idPattern.MatchString(v) {
			return fmt.Errorf("%q is not reverse-DNS, like com.example.app", v)
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Templates:")
	for i, t := range frontendTemplates {
		fmt.Fprintf(out, "  %d) %-11s %s\n", i+1, t.name, t.summary)
	}
	tmpl, err := q.ask(fmt.Sprintf("Template (1-%d or a name)", len(frontendTemplates)), cmp.Or(o.Template, defaultTemplate), func(v string) error {
		if _, ok := pickTemplate(v); !ok {
			return fmt.Errorf("no template %q; pick 1-%d or one of %s", v, len(frontendTemplates), templateNames())
		}
		return nil
	})
	if err != nil {
		return err
	}
	t, _ := pickTemplate(tmpl)

	def := "Y/n"
	if !o.Mobile {
		def = "y/N"
	}
	mobile, err := q.ask("Add iOS and Android support", def, func(v string) error {
		if _, ok := parseYesNo(v); !ok && v != def {
			return errors.New("answer y or n")
		}
		return nil
	})
	if err != nil {
		return err
	}
	if mobile != def {
		o.Mobile, _ = parseYesNo(mobile)
	}

	o.Dir, o.Name, o.ID, o.Template = dir, name, id, t.name
	return nil
}

// questioner reads answers line by line.
type questioner struct {
	in  *bufio.Reader
	out io.Writer
}

// ask prints "question [def]: " and returns the trimmed answer, or def for
// an empty one, once check accepts it.
func (q *questioner) ask(question, def string, check func(string) error) (string, error) {
	for {
		fmt.Fprintf(q.out, "%s [%s]: ", question, def)
		line, err := q.in.ReadString('\n')
		if err != nil && (err != io.EOF || line == "") {
			fmt.Fprintln(q.out)
			if err == io.EOF {
				return "", errors.New("init: no answer (input ended); nothing was created")
			}
			return "", err
		}
		v := strings.TrimSpace(line)
		if v == "" {
			v = def
		}
		if err := check(v); err != nil {
			fmt.Fprintf(q.out, "  %v\n", err)
			continue
		}
		return v, nil
	}
}

// pickTemplate finds a template by name or by its 1-based number in the
// list askInit shows.
func pickTemplate(v string) (frontendTemplate, bool) {
	if n, err := strconv.Atoi(v); err == nil {
		if n >= 1 && n <= len(frontendTemplates) {
			return frontendTemplates[n-1], true
		}
		return frontendTemplate{}, false
	}
	return findTemplate(strings.ToLower(v))
}

// parseYesNo reads y, yes, n or no, in any case.
func parseYesNo(v string) (yes, ok bool) {
	switch strings.ToLower(v) {
	case "y", "yes":
		return true, true
	case "n", "no":
		return false, true
	}
	return false, false
}

// projectConfig is tan.json as init writes it: cfg.Marshal, plus an empty
// "permissions" list (omitted by Marshal) so a new project shows where the
// mobile permissions go. It sits before "ios", or last without one.
func projectConfig(cfg *Config) []byte {
	b := cfg.Marshal()
	if i := bytes.Index(b, []byte("\n  \"ios\": ")); i >= 0 {
		return slices.Concat(b[:i+1], []byte("  \"permissions\": [],\n"), b[i+1:])
	}
	i := bytes.LastIndex(b, []byte("\n}"))
	return slices.Concat(b[:i], []byte(",\n  \"permissions\": []"), b[i:])
}

func builtDist(f Frontend) string {
	if path.Clean(f.Dist) == "." {
		return ""
	}
	return path.Join(f.Dir, f.Dist)
}

func goMod(module, local string) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "module %s\n\ngo 1.27\n", module)
	if local != "" {
		fmt.Fprintf(&b, "\nrequire github.com/tangoui/tango v0.0.0\n\nreplace github.com/tangoui/tango => %s\n", filepath.ToSlash(local))
	}
	return b.Bytes()
}

// renderTree copies the embedded directory root into dir.
func renderTree(root, dir string, data templateData) error {
	return fs.WalkDir(templates, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		if rel == "" {
			return nil
		}
		out := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		b, err := templates.ReadFile(p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(out, ".tmpl") {
			out = strings.TrimSuffix(out, ".tmpl")
			if filepath.Base(out) == "gitignore" {
				out = filepath.Join(filepath.Dir(out), ".gitignore")
			}
			t, err := template.New(rel).Delims("[[", "]]").Option("missingkey=error").Parse(string(b))
			if err != nil {
				return err
			}
			var buf bytes.Buffer
			if err := t.Execute(&buf, data); err != nil {
				return err
			}
			b = buf.Bytes()
		}
		return os.WriteFile(out, b, 0o644)
	})
}

// defaultIcon draws the 1024×1024 placeholder master icon: full bleed (no
// margin, no rounded corners; desktop shapes are derived from it, see
// icon_desktop.go), a gradient in a colour derived from name, and two
// overlapping circles (a tango).
func defaultIcon(name string) []byte {
	const n = 1024
	var h uint32 = 2166136261
	for _, c := range []byte(name) {
		h = (h ^ uint32(c)) * 16777619
	}
	hue := float64(h%360) / 360
	top, bottom := hsl(hue, 0.6, 0.55), hsl(hue+0.08, 0.65, 0.38)
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		t := float64(y) / n
		bg := color.NRGBA{
			R: uint8(float64(top.R)*(1-t) + float64(bottom.R)*t),
			G: uint8(float64(top.G)*(1-t) + float64(bottom.G)*t),
			B: uint8(float64(top.B)*(1-t) + float64(bottom.B)*t),
			A: 255,
		}
		for x := range n {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			c := bg
			c1 := circleCoverage(fx, fy, 412, 512, 230)
			c2 := circleCoverage(fx, fy, 612, 512, 230)
			if w := math.Max(c1, c2) * 0.85; w > 0 {
				c = mix(c, color.NRGBA{255, 255, 255, 255}, w)
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func circleCoverage(x, y, cx, cy, r float64) float64 {
	return clamp01(r - math.Hypot(x-cx, y-cy) + 0.5)
}

func clamp01(v float64) float64 { return math.Min(math.Max(v, 0), 1) }

func mix(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

func hsl(h, s, l float64) color.NRGBA {
	h -= math.Floor(h)
	q := l + s - l*s
	if l < 0.5 {
		q = l * (1 + s)
	}
	p := 2*l - q
	ch := func(t float64) uint8 {
		t -= math.Floor(t)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(v * 255))
	}
	return color.NRGBA{ch(h + 1.0/3), ch(h), ch(h - 1.0/3), 255}
}
