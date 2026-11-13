package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestProjectConfigMobileDefaults(t *testing.T) {
	for _, mobile := range []bool{true, false} {
		dir := filepath.Join(t.TempDir(), "app")
		if err := createProject(initOptions{Dir: dir, Template: "vanilla", Mobile: mobile}); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(filepath.Join(dir, ConfigFile))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseConfig(raw); err != nil {
			t.Fatalf("mobile=%v: generated tan.json does not parse: %v\n%s", mobile, err, raw)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if string(m["permissions"]) != "[]" {
			t.Errorf("mobile=%v: permissions = %s, want []\n%s", mobile, m["permissions"], raw)
		}
		_, hasIOS := m["ios"]
		_, hasAndroid := m["android"]
		if hasIOS != mobile || hasAndroid != mobile {
			t.Errorf("mobile=%v: ios %v, android %v\n%s", mobile, hasIOS, hasAndroid, raw)
		}
		if !mobile {
			continue
		}
		if got := compactJSON(t, m["ios"]); got != `{"minVersion":"17.0"}` {
			t.Errorf("ios = %s", got)
		}
		if got := compactJSON(t, m["android"]); got != `{"minSdk":24}` {
			t.Errorf("android = %s", got)
		}
		for _, k := range []string{"signIdentity", "provisioningProfile", "keystore", "keyAlias"} {
			if bytes.Contains(raw, []byte(k)) {
				t.Errorf("signing field %s written:\n%s", k, raw)
			}
		}
		if i, j := bytes.Index(raw, []byte(`"permissions"`)), bytes.Index(raw, []byte(`"ios"`)); i > j {
			t.Errorf("permissions after ios:\n%s", raw)
		}
	}
}

func compactJSON(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestUITemplateHasNoFrontend(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "drawn")
	if err := createProject(initOptions{Dir: dir, Template: "ui", Mobile: true}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, ConfigFile))
	if bytes.Contains(raw, []byte(`"frontend"`)) {
		t.Errorf("ui tan.json has a frontend section:\n%s", raw)
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	for _, want := range []string{"uihost.Open(", "uihost.Persist(&st)", "ui.Segmented(", "ui.Form(", "f.Validate()", "ui.Table(", "ui.Modal(", "ui.Message(", `ID: "com.example.drawn"`} {
		if !strings.Contains(string(main), want) {
			t.Errorf("main.go lacks %q", want)
		}
	}
	if strings.Contains(string(main), "go:embed") {
		t.Error("ui main.go embeds a front end")
	}
}

func TestHybridTemplate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "mixed")
	if err := createProject(initOptions{Dir: dir, Template: "hybrid", Local: repoRoot(t)}); err != nil {
		t.Fatal(err)
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	for _, want := range []string{"uihost.Attach(", "Above: true, PassThrough: true", `"page:clicked"`, `"panel:message"`, "//go:embed all:frontend/dist"} {
		if !strings.Contains(string(main), want) {
			t.Errorf("main.go lacks %q", want)
		}
	}
	ts, _ := os.ReadFile(filepath.Join(dir, "frontend", "src", "main.ts"))
	for _, want := range []string{`emit("page:clicked"`, `on("panel:message"`} {
		if !strings.Contains(string(ts), want) {
			t.Errorf("main.ts lacks %q", want)
		}
	}
}

func TestShadcnTemplate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Shiny App")
	if err := createProject(initOptions{Dir: dir, Template: "shadcn", Local: repoRoot(t)}); err != nil {
		t.Fatal(err)
	}
	fe := filepath.Join(dir, "frontend")
	for _, f := range []string{"components.json", "vite.config.ts", "tsconfig.json", "tsconfig.app.json", "src/lib/utils.ts", "src/index.css",
		"src/components/ui/button.tsx", "src/components/ui/dialog.tsx", "src/components/ui/dropdown-menu.tsx",
		"src/components/ui/select.tsx", "src/components/ui/sonner.tsx", "src/components/ui/input.tsx", "dist/.gitkeep"} {
		if _, err := os.Stat(filepath.Join(fe, f)); err != nil {
			t.Error(err)
		}
	}
	var comps struct {
		Tailwind struct{ CSS, Config string }
		Aliases  map[string]string
	}
	readJSON(t, filepath.Join(fe, "components.json"), &comps)
	if comps.Tailwind.CSS != "src/index.css" || comps.Tailwind.Config != "" || comps.Aliases["utils"] != "@/lib/utils" {
		t.Errorf("components.json = %+v", comps)
	}
	vite, _ := os.ReadFile(filepath.Join(fe, "vite.config.ts"))
	if !strings.Contains(string(vite), `"@": path.resolve(__dirname, "./src")`) || !strings.Contains(string(vite), "tailwindcss()") {
		t.Errorf("vite.config.ts:\n%s", vite)
	}
	for _, f := range []string{"tsconfig.json", "tsconfig.app.json"} {
		b, _ := os.ReadFile(filepath.Join(fe, f))
		if !strings.Contains(string(b), `"@/*": ["./src/*"]`) {
			t.Errorf("%s has no @/* path:\n%s", f, b)
		}
	}
	var pkg struct {
		Name                          string
		Dependencies, DevDependencies map[string]string
	}
	readJSON(t, filepath.Join(fe, "package.json"), &pkg)
	if pkg.Name != "shiny-app-frontend" {
		t.Errorf("package name %q", pkg.Name)
	}
	exact := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	for _, deps := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for name, v := range deps {
			if name == "@tangoui/runtime" {
				if !strings.HasPrefix(v, "file:") {
					t.Errorf("runtime = %q", v)
				}
				continue
			}
			if !exact.MatchString(v) {
				t.Errorf("%s is %q, not an exact version", name, v)
			}
		}
	}
	app, _ := os.ReadFile(filepath.Join(fe, "src", "App.tsx"))
	for _, want := range []string{"[--tango-drag:drag]", "[--tango-drag:no-drag]", "Shiny App", "<Toaster"} {
		if !strings.Contains(string(app), want) {
			t.Errorf("App.tsx lacks %q", want)
		}
	}
	main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if !strings.Contains(string(main), "tango.TitleBarHidden") || !strings.Contains(string(main), `"runtime"`) {
		t.Errorf("main.go has no hidden title bar:\n%s", main)
	}
	// The other templates keep the ordinary title bar.
	other := filepath.Join(t.TempDir(), "plain")
	if err := createProject(initOptions{Dir: other, Template: "react-ts"}); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(other, "main.go")); strings.Contains(string(b), "TitleBar") {
		t.Errorf("react-ts main.go hides the title bar:\n%s", b)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestAskInit(t *testing.T) {
	base := t.TempDir()
	busy := filepath.Join(base, "busy")
	os.MkdirAll(busy, 0o755)
	os.WriteFile(filepath.Join(busy, "x"), nil, 0o644)
	target := filepath.Join(base, "My Tool")

	tests := []struct {
		name    string
		start   initOptions
		answers string
		want    initOptions
		prompts []string // substrings expected in the output
	}{
		{
			name:    "defaults",
			start:   initOptions{Template: defaultTemplate, Mobile: true},
			answers: target + "\n\n\n\n\n",
			want:    initOptions{Dir: target, Name: "My Tool", ID: "com.example.mytool", Template: "vanilla", Mobile: true},
			prompts: []string{"Project directory [tango-app]", "App name [My Tool]", "App id (reverse-DNS) [com.example.mytool]", "7) hybrid", "Add iOS and Android support [Y/n]"},
		},
		{
			name:  "answers and retries",
			start: initOptions{Template: defaultTemplate, Mobile: true},
			// non-empty dir, then a good one; a name; a bad id, then a
			// good one; a bad template, then a number; an unclear yes/no.
			answers: busy + "\n" + target + "\n  Fancy App  \nnot-an-id\norg.acme.fancy\nsvelte\n5\nmaybe\nNO\n",
			want:    initOptions{Dir: target, Name: "Fancy App", ID: "org.acme.fancy", Template: "shadcn", Mobile: false},
			prompts: []string{"exists and is not empty", "not reverse-DNS", `no template "svelte"`, "answer y or n", "App id (reverse-DNS) [com.example.fancyapp]"},
		},
		{
			name:    "flags are defaults",
			start:   initOptions{Name: "Given", ID: "com.given.app", Template: "ui", Mobile: false},
			answers: target + "\n\n\nhybrid\n\n",
			want:    initOptions{Dir: target, Name: "Given", ID: "com.given.app", Template: "hybrid", Mobile: false},
			prompts: []string{"App name [Given]", "Template (1-7 or a name) [ui]", "[y/N]"},
		},
		{
			name:    "last line without newline",
			start:   initOptions{Template: defaultTemplate, Mobile: true},
			answers: target + "\nN\ncom.n.n\nUI\ny",
			want:    initOptions{Dir: target, Name: "N", ID: "com.n.n", Template: "ui", Mobile: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := tt.start
			var out bytes.Buffer
			if err := askInit(strings.NewReader(tt.answers), &out, &o); err != nil {
				t.Fatalf("%v\n%s", err, out.String())
			}
			if o != tt.want {
				t.Errorf("got %+v\nwant %+v", o, tt.want)
			}
			for _, p := range tt.prompts {
				if !strings.Contains(out.String(), p) {
					t.Errorf("output lacks %q:\n%s", p, out.String())
				}
			}
		})
	}

	o := initOptions{Template: defaultTemplate}
	var out bytes.Buffer
	if err := askInit(strings.NewReader(target+"\n"), &out, &o); err == nil || !strings.Contains(err.Error(), "nothing was created") {
		t.Errorf("input ending early: err = %v", err)
	}
}

func TestRunInitWithoutDir(t *testing.T) {
	t.Chdir(t.TempDir())
	old := stdinIsTerminal
	defer func() { stdinIsTerminal = old }()
	stdinIsTerminal = func() bool { return false }

	err := runInit(nil)
	if err == nil || !strings.Contains(err.Error(), "-yes") || !strings.Contains(err.Error(), "terminal") {
		t.Errorf("no dir, no terminal: err = %v", err)
	}
	if _, err := os.Stat(defaultInitDir); err == nil {
		t.Error("created a project anyway")
	}
	if err := runInit([]string{"a", "b"}); err == nil {
		t.Error("two directories accepted")
	}
}

func TestRunInitYes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go mod tidy")
	}
	root := repoRoot(t)
	t.Chdir(t.TempDir())
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	if err := runInit([]string{"-yes", "-template", "ui", "-local", root}); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(defaultInitDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != defaultInitDir || cfg.ID != "com.example.tangoapp" || cfg.HasFrontend() || cfg.IOS.MinVersion != "17.0" || cfg.Android.MinSdk != 24 {
		t.Errorf("config = %+v", cfg)
	}
}

func TestPickTemplate(t *testing.T) {
	if len(frontendTemplates) != 7 {
		t.Fatalf("%d templates, the wizard text says 7", len(frontendTemplates))
	}
	for i, tt := range frontendTemplates {
		if got, ok := pickTemplate(strings.ToUpper(tt.name)); !ok || got.name != tt.name {
			t.Errorf("by name %q: %v %v", tt.name, got.name, ok)
		}
		if got, ok := pickTemplate(string(rune('1' + i))); !ok || got.name != tt.name {
			t.Errorf("by number %d: %v %v", i+1, got.name, ok)
		}
	}
	for _, bad := range []string{"0", "8", "-1", "", "svelte"} {
		if _, ok := pickTemplate(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}
