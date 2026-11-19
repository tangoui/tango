package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDesktopHelpersPackage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX shell")
	}
	t.Setenv("GOFLAGS", "")
	cfg, err := ParseConfig([]byte(`{"name":"Desk","id":"test.example.desk","version":"1.0.0","desktopBuildTags":["production","nocoredrivers"],"desktopHelpers":[{"name":"helper","build":"printf '%s/%s' \"$TAN_TARGET_OS\" \"$TAN_TARGET_ARCH\" > \"$TAN_HELPER_OUTPUT\""}]}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.dir = t.TempDir()
	if err := os.WriteFile(filepath.Join(cfg.dir, "main.go"), []byte("//go:build production && nocoredrivers && tango_release && !tango_dev\n\npackage main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.dir, "go.mod"), []byte("module test.example/desk\ngo 1.25.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, goos := range []string{"linux", "windows"} {
		t.Run(goos, func(t *testing.T) {
			target := target{OS: goos, Arch: runtime.GOARCH}
			a, err := buildTarget(context.Background(), cfg, target, filepath.Join(cfg.dir, goos), false, featureAll)
			if err != nil {
				t.Fatal(err)
			}
			if goos == "windows" {
				script := nsisScript(cfg, a, "setup.exe")
				if !strings.Contains(script, `SetOutPath "$INSTDIR\cli"`) || !strings.Contains(script, `Delete "$INSTDIR\cli\helper.exe"`) {
					t.Fatal("installer does not install and remove the helper")
				}
				layout := filepath.Join(cfg.dir, "msix")
				if err := writeMSIXLayout(cfg, a, layout); err != nil {
					t.Fatal(err)
				}
				checkHelper(t, filepath.Join(layout, "cli", "helper.exe"), target.String())
				return
			}
			files, err := linuxSystemFiles(cfg, a, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range files {
				if file.path == "/usr/lib/Desk/helper" && file.mode == 0o755 && string(file.data) == target.String() {
					found = true
				}
			}
			if !found {
				t.Fatal("Linux system package is missing its executable helper")
			}
			archive, err := linuxTarball(cfg, a, nil)
			if err != nil {
				t.Fatal(err)
			}
			entries := readHelperTar(t, archive)
			if entries["cli/helper"] != target.String() || !strings.Contains(entries["install.sh"], `"$PREFIX/lib/Desk/helper"`) {
				t.Fatal("Linux tarball does not carry and install the helper")
			}
			t.Setenv("PATH", t.TempDir())
			appImage, err := linuxAppImage(cfg, a, nil)
			if err != nil {
				t.Fatal(err)
			}
			checkHelper(t, filepath.Join(appImage[0], "usr", "bin", "cli", "helper"), target.String())
			flatpak, err := packageFlatpak(context.Background(), cfg, a, nil)
			if err != nil {
				t.Fatal(err)
			}
			checkHelper(t, filepath.Join(filepath.Dir(flatpak[0]), "files", "lib", cfg.Exe(), "helper"), target.String())
			manifest, err := os.ReadFile(flatpak[0])
			if err != nil || !strings.Contains(string(manifest), "install -Dm755 lib/Desk/helper ${FLATPAK_DEST}/lib/Desk/helper") {
				t.Fatalf("Flatpak does not install the helper: %v", err)
			}
		})
	}
}

func TestLinuxHelperDirectoryConflict(t *testing.T) {
	cfg, err := ParseConfig([]byte(`{"name":"CLI","id":"test.example.cli","version":"1.0.0","desktopHelpers":[{"name":"helper","build":"unused"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg.dir = t.TempDir()
	_, err = build(context.Background(), cfg, buildOptions{
		targets: []target{{OS: "linux", Arch: "amd64"}}, out: "build", features: featureAll,
	})
	if err == nil || !strings.Contains(err.Error(), `executable name "CLI" conflicts with the "cli" helper directory`) {
		t.Fatalf("expected a Linux helper directory conflict, got %v", err)
	}
}

func checkHelper(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != want {
		t.Fatalf("helper %s: %q, %v; want %q", path, data, err, want)
	}
}

func readHelperTar(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entries := map[string]string{}
	archive := tar.NewReader(reader)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatal(err)
		}
		_, relative, _ := strings.Cut(header.Name, "/")
		if relative == "install.sh" || relative == "cli/helper" {
			data, err := io.ReadAll(archive)
			if err != nil {
				t.Fatal(err)
			}
			entries[relative] = string(data)
		}
	}
}
