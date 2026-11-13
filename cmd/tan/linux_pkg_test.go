package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// linuxPkgFixture is a project with icons, a URL scheme and a private MIME
// type, and a fake linux/amd64 executable.
func linuxPkgFixture(t *testing.T) (*Config, artifact) {
	cfg := testConfig(t)
	cfg.Icon = "icon.png"
	os.WriteFile(cfg.Path("icon.png"), defaultIcon("x"), 0o644)
	cfg.URLSchemes = []string{"hello"}
	cfg.FileAssociations = []FileAssociation{{Ext: "hlo", Name: "Hello document"}}
	cfg.Version = "1.2.0-rc.1"
	cfg.Linux = Linux{
		Maintainer:  "Jane Doe <jane@example.com>",
		Description: "Says hello\nA longer text.\n\nSecond paragraph.",
		Homepage:    "https://example.com",
		Depends:     []string{"xdg-utils"},
		RPMRequires: []string{"xdg-utils >= 1.1"},
	}
	a := artifact{target: target{"linux", "amd64"}, dir: filepath.Join(cfg.dir, "build", "linux-amd64")}
	os.MkdirAll(a.dir, 0o755)
	a.exe = filepath.Join(a.dir, "HelloApp")
	a.path = a.exe
	os.WriteFile(a.exe, []byte("\x7fELF fake program"), 0o755)
	t.Setenv("PATH", t.TempDir()) // no appimagetool, flatpak-builder
	t.Setenv("SOURCE_DATE_EPOCH", "1700000000")
	return cfg, a
}

func TestLinuxFormatsAndNames(t *testing.T) {
	cfg := testConfig(t)
	lin := []target{{"linux", "amd64"}, {"linux", "arm64"}}
	if got := linuxFormats(" deb,RPM,deb,,flatpak "); !slices.Equal(got, []string{"deb", "rpm", "flatpak"}) {
		t.Errorf("linuxFormats = %v", got)
	}
	if err := checkPackageFormat(cfg, "deb,rpm,flatpak,tar,appimage", lin); err != nil {
		t.Error(err)
	}
	if err := checkPackageFormat(cfg, "deb", []target{{"linux", "amd64"}, {"windows", "amd64"}}); err == nil {
		t.Error("deb for windows accepted")
	}
	if err := checkPackageFormat(cfg, "deb,snap", lin); err == nil || !strings.Contains(err.Error(), "snap") {
		t.Errorf("unknown format: %v", err)
	}
	for in, want := range map[string]string{"1.2.3": "1.2.3", "1.2.3-rc.1": "1.2.3~rc.1", "1.0.0-a-b+build-7": "1.0.0~a~b+build.7"} {
		if got := linuxPackageVersion(in); got != want {
			t.Errorf("linuxPackageVersion(%q) = %q, want %q", in, got, want)
		}
	}
	if got := linuxPackageName(cfg); got != "hello-app" {
		t.Errorf("package name %q", got)
	}
	cfg.Name = "X"
	if got := linuxPackageName(cfg); got != "com.example.hello" {
		t.Errorf("short name: package name %q", got)
	}
	for _, bad := range []string{
		`"linux": {"package": "Hello"}`,
		`"linux": {"depends": ["a, b"]}`,
		`"linux": {"flatpak": {"finishArgs": ["filesystem=home"]}}`,
	} {
		_, err := ParseConfig([]byte(`{"name": "A", "id": "com.example.a", "version": "1.0.0", ` + bad + `}`))
		if err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	c, err := ParseConfig([]byte(`{"name": "A", "id": "com.example.a", "version": "1.0.0", "linux": {"maintainer": "J <j@example.com>", "depends": ["libfoo1 (>= 2)"]}}`))
	if err != nil || c.Linux.Maintainer == "" {
		t.Errorf("linux config: %v", err)
	}
}

// readAr is a minimal ar(5) reader: the member names and data, in order.
func readAr(t *testing.T, b []byte) (names []string, data [][]byte) {
	t.Helper()
	if !bytes.HasPrefix(b, []byte("!<arch>\n")) {
		t.Fatalf("no ar magic: %q", b[:min(8, len(b))])
	}
	b = b[8:]
	for len(b) > 0 {
		if len(b) < 60 || string(b[58:60]) != "`\n" {
			t.Fatalf("bad ar header %q", b[:min(60, len(b))])
		}
		name := strings.TrimRight(string(b[:16]), " ")
		size, err := strconv.Atoi(strings.TrimSpace(string(b[48:58])))
		if err != nil {
			t.Fatal(err)
		}
		if mode := strings.TrimSpace(string(b[40:48])); mode != "100644" {
			t.Errorf("%s: mode %s", name, mode)
		}
		names = append(names, name)
		data = append(data, b[60:60+size])
		b = b[60+size+size%2:]
	}
	return names, data
}

// readTarGz returns the regular files and the directory names of a tar.gz.
func readTarGz(t *testing.T, b []byte) (map[string][]byte, map[string]int64, []string) {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	files, modes := map[string][]byte{}, map[string]int64{}
	var dirs []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Uname != "root" || h.Uid != 0 {
			t.Errorf("%s owned by %s/%d", h.Name, h.Uname, h.Uid)
		}
		if h.Typeflag == tar.TypeDir {
			dirs = append(dirs, h.Name)
			continue
		}
		data, _ := io.ReadAll(tr)
		files[h.Name], modes[h.Name] = data, h.Mode
	}
	return files, modes, dirs
}

func TestPackageDeb(t *testing.T) {
	cfg, a := linuxPkgFixture(t)
	made, err := packageLinuxFormats(context.Background(), cfg, a, []string{"deb"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cfg.dir, "build", "HelloApp-1.2.0-rc.1-linux-amd64.deb")
	if !slices.Equal(made, []string{want}) {
		t.Fatalf("made %v", made)
	}
	b, _ := os.ReadFile(want)
	names, data := readAr(t, b)
	if !slices.Equal(names, []string{"debian-binary", "control.tar.gz", "data.tar.gz"}) {
		t.Fatalf("members %v", names)
	}
	if string(data[0]) != "2.0\n" {
		t.Errorf("debian-binary %q", data[0])
	}

	ctl, ctlModes, _ := readTarGz(t, data[1])
	control := string(ctl["./control"])
	fields := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(control))
	var last string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, " ") {
			fields[last] += "\n" + line
			continue
		}
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			t.Errorf("control line %q", line)
		}
		fields[k], last = v, k
	}
	for k, v := range map[string]string{
		"Package":      "hello-app",
		"Version":      "1.2.0~rc.1",
		"Architecture": "amd64",
		"Maintainer":   "Jane Doe <jane@example.com>",
		"Depends":      "libc6, libgtk-3-0, libwebkit2gtk-4.1-0, xdg-utils",
		"Homepage":     "https://example.com",
		"Description":  "Says hello\n A longer text.\n .\n Second paragraph.",
		"Section":      "misc",
		"Priority":     "optional",
	} {
		if fields[k] != v {
			t.Errorf("control %s = %q, want %q", k, fields[k], v)
		}
	}
	if n, err := strconv.Atoi(fields["Installed-Size"]); err != nil || n < 1 {
		t.Errorf("Installed-Size %q", fields["Installed-Size"])
	}
	for _, s := range []string{"./postinst", "./postrm"} {
		if ctlModes[s] != 0o755 || !strings.HasPrefix(string(ctl[s]), "#!/bin/sh\n") {
			t.Errorf("%s: mode %o, %q", s, ctlModes[s], ctl[s])
		}
		for _, cmd := range []string{"update-desktop-database", "update-mime-database", "gtk-update-icon-cache"} {
			if !strings.Contains(string(ctl[s]), "command -v "+cmd) {
				t.Errorf("%s does not check for %s", s, cmd)
			}
		}
	}

	files, modes, dirs := readTarGz(t, data[2])
	wantFiles := []string{
		"./usr/bin/HelloApp",
		"./usr/share/applications/com.example.hello.desktop",
		"./usr/share/icons/hicolor/256x256/apps/HelloApp.png",
		"./usr/share/icons/hicolor/48x48/apps/HelloApp.png",
		"./usr/share/mime/packages/com.example.hello.xml",
		"./usr/share/doc/hello-app/copyright",
	}
	for _, f := range wantFiles {
		if files[f] == nil {
			t.Errorf("data.tar.gz has no %s (has %v)", f, sortedKeys(files))
		}
	}
	if modes["./usr/bin/HelloApp"] != 0o755 || modes["./usr/share/applications/com.example.hello.desktop"] != 0o644 {
		t.Errorf("modes %v", modes)
	}
	if !strings.Contains(string(files["./usr/share/applications/com.example.hello.desktop"]), "MimeType=x-scheme-handler/hello;application/x-helloapp-hlo;") {
		t.Errorf("desktop entry %s", files["./usr/share/applications/com.example.hello.desktop"])
	}
	for _, d := range []string{"./", "./usr/", "./usr/bin/", "./usr/share/icons/hicolor/48x48/apps/"} {
		if !slices.Contains(dirs, d) {
			t.Errorf("no directory %s in %v", d, dirs)
		}
	}
	// md5sums: one line per file, paths without the leading "./".
	sums := string(ctl["./md5sums"])
	for name, b := range files {
		line := fmt.Sprintf("%x  %s\n", md5.Sum(b), strings.TrimPrefix(name, "./"))
		if !strings.Contains(sums, line) {
			t.Errorf("md5sums lacks %q", line)
		}
	}
	if strings.Count(sums, "\n") != len(files) {
		t.Errorf("md5sums has %d lines for %d files", strings.Count(sums, "\n"), len(files))
	}
}

// rpmParsed is a header read back: tag → (type, count, data from offset).
type rpmParsed map[uint32]struct {
	typ, count uint32
	data       []byte
}

// readRPMHeader parses a header at b and returns it and its length.
func readRPMHeader(t *testing.T, b []byte, region uint32) (rpmParsed, int) {
	t.Helper()
	if !bytes.HasPrefix(b, rpmHeaderMagic) {
		t.Fatalf("no header magic: % x", b[:8])
	}
	il := int(binary.BigEndian.Uint32(b[8:]))
	dl := int(binary.BigEndian.Uint32(b[12:]))
	index := b[16 : 16+il*16]
	store := b[16+il*16 : 16+il*16+dl]
	h := rpmParsed{}
	var prevOff uint32
	for i := range il {
		e := index[i*16:]
		tag, typ, off, count := binary.BigEndian.Uint32(e), binary.BigEndian.Uint32(e[4:]), binary.BigEndian.Uint32(e[8:]), binary.BigEndian.Uint32(e[12:])
		if i == 0 {
			// The region tag and its trailer, the last 16 bytes of the store.
			if tag != region || typ != rpmTypeBin || count != 16 || int(off) != dl-16 {
				t.Fatalf("region entry %d %d %d %d (dl %d)", tag, typ, off, count, dl)
			}
			tr := store[off:]
			if binary.BigEndian.Uint32(tr) != region || int32(binary.BigEndian.Uint32(tr[8:])) != -int32(il*16) {
				t.Errorf("region trailer % x", tr)
			}
			continue
		}
		if off < prevOff {
			t.Errorf("tag %d: data offsets not increasing", tag)
		}
		prevOff = off
		if (typ == rpmTypeInt32 && off%4 != 0) || (typ == rpmTypeInt16 && off%2 != 0) {
			t.Errorf("tag %d misaligned at %d", tag, off)
		}
		h[tag] = struct {
			typ, count uint32
			data       []byte
		}{typ, count, store[off:]}
	}
	return h, 16 + il*16 + dl
}

func (h rpmParsed) strs(tag uint32) []string {
	e, ok := h[tag]
	if !ok {
		return nil
	}
	parts := strings.Split(string(e.data), "\x00")
	return parts[:e.count]
}

func (h rpmParsed) int32s(tag uint32) []uint32 {
	e := h[tag]
	var vs []uint32
	for i := range e.count {
		vs = append(vs, binary.BigEndian.Uint32(e.data[i*4:]))
	}
	return vs
}

// readCpio returns the files of a newc archive.
func readCpio(t *testing.T, b []byte) (map[string][]byte, map[string]uint32) {
	t.Helper()
	files, modes := map[string][]byte{}, map[string]uint32{}
	for {
		if string(b[:6]) != "070701" {
			t.Fatalf("cpio magic %q", b[:6])
		}
		field := func(i int) int {
			n, err := strconv.ParseUint(string(b[6+i*8:14+i*8]), 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			return int(n)
		}
		mode, size, namesize := field(1), field(6), field(11)
		name := string(b[110 : 110+namesize-1])
		off := (110 + namesize + 3) &^ 3
		if name == "TRAILER!!!" {
			return files, modes
		}
		files[name], modes[name] = b[off:off+size], uint32(mode)
		b = b[(off+size+3)&^3:]
	}
}

func TestPackageRPM(t *testing.T) {
	cfg, a := linuxPkgFixture(t)
	made, err := packageLinuxFormats(context.Background(), cfg, a, []string{"rpm"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(made[0])
	if err != nil || filepath.Base(made[0]) != "HelloApp-1.2.0-rc.1-linux-amd64.rpm" {
		t.Fatalf("made %v: %v", made, err)
	}
	// Lead.
	if !bytes.HasPrefix(b, rpmLeadMagic) || b[4] != 3 || binary.BigEndian.Uint16(b[78:]) != 5 {
		t.Fatalf("lead % x", b[:96])
	}
	if name := string(bytes.TrimRight(b[10:76], "\x00")); name != "hello-app-1.2.0~rc.1-1" {
		t.Errorf("lead name %q", name)
	}
	sig, n := readRPMHeader(t, b[96:], rpmTagHeaderSignatures)
	start := 96 + (n+7)&^7
	h, n := readRPMHeader(t, b[start:], rpmTagHeaderImmutable)
	header, payload := b[start:start+n], b[start+n:]

	if got := sig.int32s(rpmSigTagSize); len(got) != 1 || int(got[0]) != len(header)+len(payload) {
		t.Errorf("signature size %v, want %d", got, len(header)+len(payload))
	}
	sum := sha256.Sum256(header)
	if got := sig.strs(rpmSigTagSHA256); len(got) != 1 || got[0] != hex.EncodeToString(sum[:]) {
		t.Errorf("header SHA-256 %v", got)
	}
	m := md5.Sum(append(slices.Clone(header), payload...))
	if e := sig[rpmSigTagMD5]; e.typ != rpmTypeBin || !bytes.Equal(e.data[:16], m[:]) {
		t.Errorf("MD5 mismatch")
	}

	for tag, want := range map[uint32]string{
		rpmTagName: "hello-app", rpmTagVersion: "1.2.0~rc.1", rpmTagRelease: "1", rpmTagArch: "x86_64",
		rpmTagOS: "linux", rpmTagSummary: "Says hello", rpmTagPayloadFormat: "cpio", rpmTagPayloadCompressor: "gzip",
		rpmTagURL: "https://example.com", rpmTagPackager: "Jane Doe <jane@example.com>",
		rpmTagSourceRPM: "hello-app-1.2.0~rc.1-1.src.rpm",
	} {
		if got := h.strs(tag); len(got) != 1 || got[0] != want {
			t.Errorf("tag %d = %q, want %q", tag, got, want)
		}
	}
	if e := h[rpmTagPostInProg]; e.typ != rpmTypeStringArray {
		t.Errorf("POSTINPROG type %d", e.typ)
	}
	reqs := h.strs(rpmTagRequireName)
	for _, r := range []string{"libgtk-3.so.0()(64bit)", "libwebkit2gtk-4.1.so.0()(64bit)", "xdg-utils", "/bin/sh"} {
		if !slices.Contains(reqs, r) {
			t.Errorf("requires %v lacks %s", reqs, r)
		}
	}
	if i := slices.Index(reqs, "xdg-utils"); i < 0 || h.strs(rpmTagRequireVersion)[i] != "1.1" || h.int32s(rpmTagRequireFlags)[i] != rpmSenseGreater|rpmSenseEqual {
		t.Errorf("xdg-utils >= 1.1 not parsed")
	}

	// The file list against the payload.
	dirs, bases, idx := h.strs(rpmTagDirNames), h.strs(rpmTagBaseNames), h.int32s(rpmTagDirIndexes)
	digests, sizes := h.strs(rpmTagFileDigests), h.int32s(rpmTagFileSizes)
	zr, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	cpio, _ := io.ReadAll(zr)
	if got := sig.int32s(rpmSigTagPayloadSize); len(got) != 1 || int(got[0]) != len(cpio) {
		t.Errorf("payload size %v, want %d", got, len(cpio))
	}
	pd := sha256.Sum256(payload)
	if got := h.strs(rpmTagPayloadDigest); len(got) != 1 || got[0] != hex.EncodeToString(pd[:]) {
		t.Errorf("payload digest %v", got)
	}
	files, modes := readCpio(t, cpio)
	if len(files) != len(bases) {
		t.Errorf("%d cpio files, %d in the header", len(files), len(bases))
	}
	for i, base := range bases {
		p := "." + dirs[idx[i]] + base
		data, ok := files[p]
		if !ok {
			t.Errorf("payload lacks %s", p)
			continue
		}
		ds := sha256.Sum256(data)
		if digests[i] != hex.EncodeToString(ds[:]) || int(sizes[i]) != len(data) {
			t.Errorf("%s: digest or size mismatch", p)
		}
	}
	if modes["./usr/bin/HelloApp"] != 0o100755 || string(files["./usr/bin/HelloApp"]) != "\x7fELF fake program" {
		t.Errorf("program in payload: mode %o", modes["./usr/bin/HelloApp"])
	}
	if files["./usr/share/applications/com.example.hello.desktop"] == nil || files["./usr/share/mime/packages/com.example.hello.xml"] == nil {
		t.Errorf("payload files %v", sortedKeys(files))
	}
}

func TestPackageFlatpak(t *testing.T) {
	cfg, a := linuxPkgFixture(t)
	cfg.Icons.Linux.Symbolic = "symbolic.svg"
	os.WriteFile(cfg.Path("symbolic.svg"), []byte("<svg/>"), 0o644)
	cfg.Linux.Flatpak.FinishArgs = []string{"--socket=pulseaudio", "--share=ipc"}
	made, err := packageLinuxFormats(context.Background(), cfg, a, []string{"flatpak"})
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(cfg.dir, "build", "HelloApp-amd64.flatpak")
	if !slices.Equal(made, []string{filepath.Join(dir, "com.example.hello.json")}) {
		t.Fatalf("made %v", made)
	}
	b, _ := os.ReadFile(made[0])
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"id": "com.example.hello", "runtime": "org.gnome.Platform", "sdk": "org.gnome.Sdk",
		"runtime-version": defaultFlatpakRuntime, "command": "HelloApp"} {
		if m[k] != v {
			t.Errorf("%s = %v, want %s", k, m[k], v)
		}
	}
	var args []string
	for _, a := range m["finish-args"].([]any) {
		args = append(args, a.(string))
	}
	if !slices.Equal(args, append(slices.Clone(flatpakFinishArgs), "--socket=pulseaudio")) {
		t.Errorf("finish-args %v", args)
	}
	if slices.ContainsFunc(args, func(s string) bool { return strings.HasPrefix(s, "--filesystem") }) {
		t.Errorf("file system access by default: %v", args)
	}
	mods := m["modules"].([]any)
	mod := mods[0].(map[string]any)
	if mod["buildsystem"] != "simple" || len(mods) != 1 {
		t.Errorf("module %v", mod)
	}
	src := mod["sources"].([]any)[0].(map[string]any)
	if src["type"] != "dir" || src["path"] != "files" {
		t.Errorf("source %v", src)
	}
	// Every install command copies a file that exists in files/ to the same
	// path under /app; exported files carry the app id.
	var dests []string
	for _, c := range mod["build-commands"].([]any) {
		f := strings.Fields(c.(string))
		if len(f) != 4 || f[0] != "install" {
			t.Fatalf("command %q", c)
		}
		if _, err := os.Stat(filepath.Join(dir, "files", f[2])); err != nil {
			t.Errorf("%q: %v", c, err)
		}
		if f[3] != "${FLATPAK_DEST}/"+f[2] {
			t.Errorf("%q installs elsewhere", c)
		}
		dests = append(dests, f[2])
	}
	for _, p := range []string{
		"bin/HelloApp",
		"share/applications/com.example.hello.desktop",
		"share/icons/hicolor/128x128/apps/com.example.hello.png",
		"share/icons/hicolor/scalable/apps/com.example.hello-symbolic.svg",
		"share/mime/packages/com.example.hello.xml",
	} {
		if !slices.Contains(dests, p) {
			t.Errorf("no %s in %v", p, dests)
		}
	}
	desktop, _ := os.ReadFile(filepath.Join(dir, "files", "share", "applications", "com.example.hello.desktop"))
	if !strings.Contains(string(desktop), "\nIcon=com.example.hello\n") || !strings.Contains(string(desktop), "Exec=HelloApp %U") {
		t.Errorf("desktop entry:\n%s", desktop)
	}
}
