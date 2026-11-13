package main

import (
	"bytes"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot is the Tango checkout this CLI lives in.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Skip("not inside a Tango checkout")
	}
	return root
}

func TestCreateProjectFiles(t *testing.T) {
	root := repoRoot(t)
	for _, ft := range frontendTemplates {
		t.Run(ft.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "Hello World")
			err := createProject(initOptions{Dir: dir, Template: ft.name, Local: root})
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := LoadConfig(dir)
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Name != "Hello World" || cfg.ID != "com.example.helloworld" || cfg.Frontend != ft.frontend {
				t.Errorf("config = %+v", cfg)
			}
			main, _ := os.ReadFile(filepath.Join(dir, "main.go"))
			if strings.Contains(string(main), "[[") {
				t.Errorf("main.go not rendered:\n%s", main)
			}
			if !cfg.HasFrontend() { // self-drawn UI: nothing to embed or build
				if _, err := os.Stat(filepath.Join(dir, "frontend")); err == nil {
					t.Error("frontend directory in a template without a front end")
				}
				return
			}
			embed := "//go:embed all:" + filepath.ToSlash(filepath.Join(cfg.Frontend.Dir, cfg.Frontend.Dist))
			if !strings.Contains(string(main), embed) || strings.Contains(string(main), "[[") {
				t.Errorf("main.go not rendered (want %q):\n%s", embed, main)
			}
			if _, err := os.Stat(cfg.DistDir()); err != nil {
				t.Errorf("embed dir missing: %v", err)
			}
			if pj, err := os.ReadFile(filepath.Join(dir, "frontend", "package.json")); err == nil {
				if !strings.Contains(string(pj), `"@tangoui/runtime": "file:`) || !strings.Contains(string(pj), `"hello-world-frontend"`) {
					t.Errorf("package.json:\n%s", pj)
				}
			} else if ft.name != "vanilla" {
				t.Error(err)
			}
			gi, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
			if ft.name == "vanilla" && strings.Contains(string(gi), "frontend") {
				t.Errorf("vanilla .gitignore hides the sources:\n%s", gi)
			}
			img, err := os.ReadFile(filepath.Join(dir, "icon.png"))
			if err != nil {
				t.Fatal(err)
			}
			if c, err := png.DecodeConfig(bytes.NewReader(img)); err != nil || c.Width != 1024 || c.Height != 1024 {
				t.Errorf("icon: %v %+v", err, c)
			}
		})
	}
}

func TestCreateProjectRejects(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x"), nil, 0o644)
	if err := createProject(initOptions{Dir: dir, Template: "vanilla"}); err == nil {
		t.Error("non-empty dir accepted")
	}
	if err := createProject(initOptions{Dir: filepath.Join(dir, "new"), Template: "svelte"}); err == nil {
		t.Error("unknown template accepted")
	}
}

// TestGeneratedProjectBuilds compiles fresh projects against this checkout,
// offline (GOPROXY=off; the module cache must hold Tango's dependencies).
func TestGeneratedProjectBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles the generated projects")
	}
	root := repoRoot(t)
	for _, name := range []string{"vanilla", "vanilla-ts", "shadcn", "ui", "hybrid"} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "demo")
			if err := createProject(initOptions{Dir: dir, Template: name, Local: root}); err != nil {
				t.Fatal(err)
			}
			build := func(dev bool) string {
				bin := filepath.Join(t.TempDir(), "app")
				cmd := exec.Command("go", "build", "-tags", buildTags(dev), "-o", bin, ".")
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off", "CGO_ENABLED=0")
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("go build: %v\n%s", err, out)
				}
				return bin
			}
			// tan dev (-tags tango_dev) hot-reloads the self-drawn
			// template; the hybrid one attaches a canvas to a WebView and
			// needs restarts.
			bin := build(true)
			if got, want := hotCapable(bin), name == "ui"; got != want {
				t.Errorf("hot reload %v, want %v (devhost %v, Attach %v)", got, want, hasDevHost(bin), usesAttach(bin))
			}
			// A production build (tan build, plain go build) has none of
			// the development features.
			if name == "ui" || name == "vanilla" {
				prod := build(false)
				if hasDevHost(prod) {
					t.Error("production build links the dev host")
				}
				if b, _ := os.ReadFile(prod); bytes.Contains(b, []byte("TANGO_DEV_URL")) || bytes.Contains(b, []byte("TANGO_AUTOMATION_TOKEN_FILE")) {
					t.Error("production build reads TANGO_DEV_URL or TANGO_AUTOMATION")
				}
			}
		})
	}
}
