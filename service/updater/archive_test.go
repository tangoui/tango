package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"v1.2.3", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
		{"1.10.0", "1.9.9", 1},
		{"2.0.0", "10.0.0", -1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		{"1.0.0-rc.1", "1.0.0-beta.11", 1},
		{"1.0.0+build.5", "1.0.0+build.9", 0},
	}
	for _, tt := range tests {
		got, err := CompareVersions(tt.a, tt.b)
		if err != nil || got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d", tt.a, tt.b, got, err, tt.want)
		}
		if back, _ := CompareVersions(tt.b, tt.a); back != -tt.want {
			t.Errorf("Compare(%q, %q) not antisymmetric", tt.b, tt.a)
		}
	}
	for _, bad := range []string{"", "v", "1.x", "1.2.3.4", "1.0.0-", "1.0.0-a..b", "-1.0"} {
		if _, err := CompareVersions(bad, "1.0.0"); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

// ---- archives and apply steps ----

type entry struct {
	name string
	mode fs.FileMode
	body string // link target for symlinks
}

func makeZip(t *testing.T, path string, entries []entry) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		h.SetMode(e.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(e.body))
	}
	zw.Close()
	os.WriteFile(path, buf.Bytes(), 0o644)
}

func makeTarGz(t *testing.T, path string, entries []entry) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: int64(e.mode.Perm())}
		switch {
		case e.mode.IsDir():
			h.Typeflag = tar.TypeDir
		case e.mode&fs.ModeSymlink != 0:
			h.Typeflag, h.Linkname = tar.TypeSymlink, e.body
		default:
			h.Typeflag, h.Size = tar.TypeReg, int64(len(e.body))
		}
		tw.WriteHeader(h)
		if h.Typeflag == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	tw.Close()
	gz.Close()
	os.WriteFile(path, buf.Bytes(), 0o644)
}

var appEntries = []entry{
	{"Notes.app/", fs.ModeDir | 0o755, ""},
	{"Notes.app/Contents/Info.plist", 0o644, "<plist/>"},
	{"Notes.app/Contents/MacOS/Notes", 0o755, "#!/bin/sh\n"},
	{"Notes.app/Contents/Frameworks/F.framework/Versions/A/F", 0o644, "lib"},
	{"Notes.app/Contents/Frameworks/F.framework/Versions/Current", fs.ModeSymlink | 0o777, "A"},
	{"Notes.app/Contents/Frameworks/F.framework/F", fs.ModeSymlink | 0o777, "Versions/Current/F"},
	{"__MACOSX/._Notes.app", 0o644, "fork"},
}

func TestExtract(t *testing.T) {
	for _, kind := range []string{"zip", "tar.gz"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "a."+kind)
			if kind == "zip" {
				makeZip(t, archive, appEntries)
			} else {
				makeTarGz(t, archive, appEntries)
			}
			dst := filepath.Join(dir, "out")
			os.Mkdir(dst, 0o755)
			if err := extract(archive, dst); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(dst, "Notes.app/Contents/Frameworks/F.framework/F"))
			if err != nil || string(b) != "lib" {
				t.Fatalf("through symlinks: %q, %v", b, err)
			}
			if runtime.GOOS != "windows" {
				fi, _ := os.Stat(filepath.Join(dst, "Notes.app/Contents/MacOS/Notes"))
				if fi.Mode().Perm()&0o100 == 0 {
					t.Fatalf("executable bit lost: %v", fi.Mode())
				}
			}
		})
	}
}

func TestExtractRejectsEscapes(t *testing.T) {
	cases := map[string][]entry{
		"dotdot":        {{"../evil", 0o644, "x"}},
		"nested dotdot": {{"a/../../evil", 0o644, "x"}},
		"absolute":      {{"/tmp/evil", 0o644, "x"}},
		"symlink out":   {{"a/link", fs.ModeSymlink | 0o777, "../../etc"}},
		"symlink abs":   {{"link", fs.ModeSymlink | 0o777, "/etc"}},
	}
	for name, entries := range cases {
		for _, kind := range []string{"zip", "tar.gz"} {
			dir := t.TempDir()
			archive := filepath.Join(dir, "a."+kind)
			if kind == "zip" {
				makeZip(t, archive, entries)
			} else {
				makeTarGz(t, archive, entries)
			}
			dst := filepath.Join(dir, "out")
			os.Mkdir(dst, 0o755)
			if err := extract(archive, dst); err == nil {
				t.Errorf("%s (%s): extracted", name, kind)
			}
			if _, err := os.Lstat(filepath.Join(dir, "evil")); err == nil {
				t.Errorf("%s (%s): wrote outside", name, kind)
			}
		}
	}
	if err := extract(filepath.Join(t.TempDir(), "a.rar"), t.TempDir()); err == nil {
		t.Error("unknown archive type accepted")
	}
}

func TestBundleOf(t *testing.T) {
	j := filepath.Join
	if b, ok := bundleOf(j("/Applications", "Notes.app", "Contents", "MacOS", "Notes")); !ok || b != j("/Applications", "Notes.app") {
		t.Errorf("bundleOf = %q, %v", b, ok)
	}
	for _, exe := range []string{j("/usr", "local", "bin", "notes"), j("/x", "Notes", "Contents", "MacOS", "n")} {
		if _, ok := bundleOf(exe); ok {
			t.Errorf("bundleOf(%q) ok", exe)
		}
	}
}
