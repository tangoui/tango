package main

import (
	"bytes"
	"context"
	"debug/pe"
	"encoding/binary"
	"encoding/xml"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"
)

func TestWinVersion(t *testing.T) {
	for in, want := range map[string][4]uint16{
		"1.2.3":         {1, 2, 3, 0},
		"v2.0":          {2, 0, 0, 0},
		"1.2.3-beta.4":  {1, 2, 3, 0},
		"1.2.3.4.5":     {1, 2, 3, 0}, // the fourth part is "4.5": not a number
		"0.1.0+build.7": {0, 1, 0, 0},
		"70000.1.1":     {65535, 1, 1, 0},
		"":              {},
	} {
		if got := winVersion(in); got != want {
			t.Errorf("winVersion(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestICO(t *testing.T) {
	frames, err := pngFrames(solidImage(300, color.NRGBA{9, 8, 7, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != len(winIconSizes) {
		t.Fatalf("%d frames", len(frames))
	}
	for i, f := range frames {
		img, err := png.Decode(bytes.NewReader(f.data))
		if err != nil {
			t.Fatal(err)
		}
		n := winIconSizes[i]
		if img.Bounds().Dx() != n {
			t.Errorf("frame %d is %v", i, img.Bounds())
		}
		if want := uint8(n % 256); f.width != want || f.height != want || f.bits != 32 || f.planes != 1 {
			t.Errorf("frame %d header %+v", i, f)
		}
	}
	back, err := parseICO(icoFile(frames))
	if err != nil {
		t.Fatal(err)
	}
	for i := range frames {
		if back[i].width != frames[i].width || !bytes.Equal(back[i].data, frames[i].data) {
			t.Errorf("frame %d did not round-trip", i)
		}
	}
	for _, bad := range [][]byte{nil, {0, 0, 2, 0, 1, 0}, {0, 0, 1, 0, 0, 0}, icoFile(frames)[:40]} {
		if _, err := parseICO(bad); err == nil {
			t.Errorf("parseICO(% x…) accepted", bad[:min(len(bad), 8)])
		}
	}
}

func TestWindowsIconSources(t *testing.T) {
	cfg := testConfig(t)
	if f, err := windowsIconFrames(cfg); f != nil || err != nil {
		t.Errorf("no icon: %v %v", f, err)
	}
	cfg = iconTestConfig(t)
	frames, err := windowsIconFrames(cfg)
	if err != nil || len(frames) != len(winIconSizes) {
		t.Fatalf("derived: %d frames, %v", len(frames), err)
	}
	big, _ := png.Decode(bytes.NewReader(frames[len(frames)-1].data))
	if _, _, _, a := big.At(0, 0).RGBA(); a != 0 {
		t.Error("derived Windows icon has no transparent margin")
	}

	// An .ico is used as is.
	own := []icoFrame{{width: 16, height: 16, planes: 1, bits: 32, data: []byte("not really a png")}}
	os.WriteFile(cfg.Path("app.ico"), icoFile(own), 0o644)
	cfg.Icons.Windows = "app.ico"
	frames, err = windowsIconFrames(cfg)
	if err != nil || len(frames) != 1 || string(frames[0].data) != "not really a png" {
		t.Errorf(".ico override: %+v %v", frames, err)
	}

	// A PNG is scaled, not reshaped.
	var buf bytes.Buffer
	png.Encode(&buf, solidImage(512, color.NRGBA{1, 2, 3, 255}))
	os.WriteFile(cfg.Path("win.png"), buf.Bytes(), 0o644)
	cfg.Icons.Windows = "win.png"
	frames, err = windowsIconFrames(cfg)
	if err != nil || len(frames) != len(winIconSizes) {
		t.Fatalf(".png override: %d frames, %v", len(frames), err)
	}
	img, _ := png.Decode(bytes.NewReader(frames[0].data))
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0xFFFF {
		t.Error(".png override was reshaped")
	}

	cfg.Icons.Windows = "missing.ico"
	if _, err := windowsIconFrames(cfg); err == nil {
		t.Error("missing .ico: no error")
	}
	// A broken icon is a warning; manifest and version are still linked.
	res := windowsResources(cfg, "amd64")
	if len(res) != 2 || res[0].typ != rtManifest || res[1].typ != rtVersion {
		t.Errorf("resources without an icon: %d", len(res))
	}
}

func TestSysoMatches(t *testing.T) {
	for name, want := range map[string]bool{
		"rsrc.syso":               true,
		"rsrc_windows.syso":       true,
		"rsrc_windows_amd64.syso": true,
		"rsrc_amd64.syso":         true,
		"rsrc_windows_386.syso":   false,
		"rsrc_arm64.syso":         false,
		"rsrc_linux.syso":         false,
		"rsrc_linux_amd64.syso":   false,
		"_rsrc.syso":              false,
		"my_resources.syso":       true,
	} {
		if got := sysoMatches(name, "windows", "amd64"); got != want {
			t.Errorf("sysoMatches(%q) = %v", name, got)
		}
	}
}

// peRes is a resource read back from a .rsrc section.
type peRes struct {
	typ, id, lang uint32
	data          []byte
}

// walkRsrc reads the resource tree of a .rsrc section whose data entries
// hold RVAs relative to base (0 for an object file, where they are still
// section offsets). It returns the resources and the offsets of the data
// entries.
func walkRsrc(t *testing.T, sect []byte, base uint32) (res []peRes, entries []uint32) {
	t.Helper()
	le := binary.LittleEndian
	var walk func(off uint32, depth int, path [3]uint32)
	walk = func(off uint32, depth int, path [3]uint32) {
		if int(off)+16 > len(sect) {
			t.Fatalf("directory at %#x outside the section", off)
		}
		if named := le.Uint16(sect[off+12:]); named != 0 {
			t.Errorf("directory at %#x has %d named entries", off, named)
		}
		ids := int(le.Uint16(sect[off+14:]))
		prev := int64(-1)
		for i := range ids {
			e := sect[off+16+8*uint32(i):]
			id, target := le.Uint32(e), le.Uint32(e[4:])
			if int64(id) <= prev {
				t.Errorf("directory at %#x: ids not ascending", off)
			}
			prev = int64(id)
			path[depth] = id
			if target&0x80000000 != 0 {
				if depth == 2 {
					t.Fatalf("language level points to a directory")
				}
				walk(target&^0x80000000, depth+1, path)
				continue
			}
			if depth != 2 {
				t.Fatalf("data entry at depth %d", depth)
			}
			rva, size := le.Uint32(sect[target:]), le.Uint32(sect[target+4:])
			o := rva - base
			if uint64(o)+uint64(size) > uint64(len(sect)) {
				t.Fatalf("resource %v data %#x+%d outside the section", path, rva, size)
			}
			res = append(res, peRes{path[0], path[1], path[2], sect[o : o+size]})
			entries = append(entries, target)
		}
	}
	walk(0, 0, [3]uint32{})
	return res, entries
}

func findPERes(res []peRes, typ, id uint32) *peRes {
	for i := range res {
		if res[i].typ == typ && res[i].id == id {
			return &res[i]
		}
	}
	return nil
}

// verNode is a decoded VS_VERSIONINFO block.
type verNode struct {
	key      string
	value    []byte
	text     bool
	children []verNode
}

func decodeVer(t *testing.T, b []byte) verNode {
	t.Helper()
	le := binary.LittleEndian
	if len(b) < 6 {
		t.Fatalf("version block truncated")
	}
	length, valueLen, typ := int(le.Uint16(b)), int(le.Uint16(b[2:])), le.Uint16(b[4:])
	if length > len(b) || length < 6 {
		t.Fatalf("version block length %d of %d", length, len(b))
	}
	b = b[:length]
	p := 6
	var key []uint16
	for ; le.Uint16(b[p:]) != 0; p += 2 {
		key = append(key, le.Uint16(b[p:]))
	}
	p += 2
	p = (p + 3) &^ 3
	n := verNode{key: string(utf16.Decode(key)), text: typ == 1}
	size := valueLen
	if n.text {
		size *= 2
	}
	n.value = b[p : p+size]
	p += size
	for {
		p = (p + 3) &^ 3
		if p >= len(b) {
			break
		}
		c := decodeVer(t, b[p:])
		n.children = append(n.children, c)
		p += int(le.Uint16(b[p:]))
	}
	return n
}

func utf16String(b []byte) string {
	var u []uint16
	for i := 0; i+1 < len(b); i += 2 {
		if v := binary.LittleEndian.Uint16(b[i:]); v != 0 {
			u = append(u, v)
		}
	}
	return string(utf16.Decode(u))
}

// checkResources verifies what tan links: the icon group, the manifest,
// the version information.
func checkResources(t *testing.T, res []peRes, cfg *Config, icons int) {
	t.Helper()
	for _, r := range res {
		if r.lang != winResLang {
			t.Errorf("resource %d/%d language %#x", r.typ, r.id, r.lang)
		}
	}
	le := binary.LittleEndian
	if icons > 0 {
		g := findPERes(res, rtGroupIcon, 1)
		if g == nil {
			t.Fatal("no RT_GROUP_ICON 1")
		}
		if le.Uint16(g.data) != 0 || le.Uint16(g.data[2:]) != 1 || int(le.Uint16(g.data[4:])) != icons || len(g.data) != 6+14*icons {
			t.Fatalf("group header % x", g.data[:6])
		}
		for i := range icons {
			e := g.data[6+14*i:]
			id := uint32(le.Uint16(e[12:]))
			ic := findPERes(res, rtIcon, id)
			if ic == nil {
				t.Fatalf("group entry %d names missing RT_ICON %d", i, id)
			}
			if int(le.Uint32(e[8:])) != len(ic.data) {
				t.Errorf("RT_ICON %d size %d, group says %d", id, len(ic.data), le.Uint32(e[8:]))
			}
			img, err := png.Decode(bytes.NewReader(ic.data))
			if err != nil {
				t.Fatalf("RT_ICON %d: %v", id, err)
			}
			want := int(e[0])
			if want == 0 {
				want = 256
			}
			if img.Bounds().Dx() != want || img.Bounds().Dy() != want {
				t.Errorf("RT_ICON %d is %v, group says %d", id, img.Bounds(), want)
			}
		}
	} else if findPERes(res, rtGroupIcon, 1) != nil {
		t.Error("icon group without an icon")
	}

	m := findPERes(res, rtManifest, 1)
	if m == nil {
		t.Fatal("no RT_MANIFEST 1")
	}
	dec := xml.NewDecoder(bytes.NewReader(m.data))
	for {
		if _, err := dec.Token(); err != nil {
			if err.Error() != "EOF" {
				t.Fatalf("manifest is not well-formed XML: %v", err)
			}
			break
		}
	}
	for _, want := range []string{
		`<dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2, PerMonitor</dpiAwareness>`,
		`name="Microsoft.Windows.Common-Controls" version="6.0.0.0"`,
		`publicKeyToken="6595b64144ccf1df"`,
		`<activeCodePage xmlns="http://schemas.microsoft.com/SMI/2019/WindowsSettings">UTF-8</activeCodePage>`,
		`<longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>`,
		`<supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>`,
		`name="com.example.hello" version="1.2.3.0"`,
	} {
		if !bytes.Contains(m.data, []byte(want)) {
			t.Errorf("manifest lacks %s", want)
		}
	}

	v := findPERes(res, rtVersion, 1)
	if v == nil {
		t.Fatal("no RT_VERSION 1")
	}
	root := decodeVer(t, v.data)
	if root.key != "VS_VERSION_INFO" || len(root.value) != 52 || le.Uint32(root.value) != 0xFEEF04BD {
		t.Fatalf("VS_VERSION_INFO %q value %d bytes", root.key, len(root.value))
	}
	if ms, ls := le.Uint32(root.value[8:]), le.Uint32(root.value[12:]); ms != 1<<16|2 || ls != 3<<16 {
		t.Errorf("file version %#x %#x", ms, ls)
	}
	strs := map[string]string{}
	var translation []byte
	for _, c := range root.children {
		switch c.key {
		case "StringFileInfo":
			if len(c.children) != 1 || c.children[0].key != "040904B0" {
				t.Fatalf("string tables %+v", c.children)
			}
			for _, s := range c.children[0].children {
				strs[s.key] = utf16String(s.value)
			}
		case "VarFileInfo":
			translation = c.children[0].value
		}
	}
	want := map[string]string{
		"FileDescription":  "Hello App",
		"ProductName":      "Hello App",
		"FileVersion":      "1.2.3.0",
		"ProductVersion":   "1.2.3",
		"LegalCopyright":   "© 2026 Example",
		"OriginalFilename": "HelloApp.exe",
		"InternalName":     "HelloApp",
	}
	for k, w := range want {
		if strs[k] != w {
			t.Errorf("version string %s = %q, want %q", k, strs[k], w)
		}
	}
	if !bytes.Equal(translation, []byte{0x09, 0x04, 0xB0, 0x04}) {
		t.Errorf("translation % x", translation)
	}
}

func winResConfig(t *testing.T, icon bool) *Config {
	var cfg *Config
	if icon {
		cfg = iconTestConfig(t)
	} else {
		cfg = testConfig(t)
	}
	cfg.Version = "1.2.3"
	cfg.Copyright = "© 2026 Example"
	return cfg
}

func TestResourceObject(t *testing.T) {
	cfg := winResConfig(t, true)
	res := windowsResources(cfg, "amd64")
	for arch, want := range map[string][2]uint16{
		"amd64": {pe.IMAGE_FILE_MACHINE_AMD64, 0x0003},
		"arm64": {pe.IMAGE_FILE_MACHINE_ARM64, 0x0002},
		"386":   {pe.IMAGE_FILE_MACHINE_I386, 0x0007},
	} {
		obj, err := resourceObject(arch, res)
		if err != nil {
			t.Fatal(err)
		}
		f, err := pe.NewFile(bytes.NewReader(obj))
		if err != nil {
			t.Fatalf("%s: debug/pe: %v", arch, err)
		}
		if f.Machine != want[0] || len(f.Sections) != 1 || f.Sections[0].Name != ".rsrc" {
			t.Fatalf("%s: machine %#x, sections %d", arch, f.Machine, len(f.Sections))
		}
		s := f.Sections[0]
		if s.Characteristics != pe.IMAGE_SCN_CNT_INITIALIZED_DATA|pe.IMAGE_SCN_MEM_READ {
			t.Errorf("%s: characteristics %#x", arch, s.Characteristics)
		}
		if len(f.COFFSymbols) != 1 || f.COFFSymbols[0].StorageClass != 3 || f.COFFSymbols[0].SectionNumber != 1 {
			t.Errorf("%s: symbols %+v", arch, f.COFFSymbols)
		}
		data, err := s.Data()
		if err != nil {
			t.Fatal(err)
		}
		got, entries := walkRsrc(t, data, 0)
		if len(got) != len(res) || len(s.Relocs) != len(res) || int(s.NumberOfRelocations) != len(res) {
			t.Fatalf("%s: %d resources, %d relocations, want %d", arch, len(got), len(s.Relocs), len(res))
		}
		for i, r := range s.Relocs {
			if r.Type != want[1] || r.SymbolTableIndex != 0 || r.VirtualAddress != entries[i] {
				t.Errorf("%s: relocation %d = %+v, want type %#x at %#x", arch, i, r, want[1], entries[i])
			}
		}
		checkResources(t, got, cfg, len(winIconSizes))
	}
	if _, err := resourceObject("riscv64", res); err == nil {
		t.Error("riscv64 accepted")
	}
}

// TestWindowsExecutable cross-compiles a minimal program for Windows the
// way tan build does, then reads the resources back from the executable.
func TestWindowsExecutable(t *testing.T) {
	if testing.Short() {
		t.Skip("cross-compiles")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go command")
	}
	for _, tc := range []struct {
		arch string
		icon bool
	}{{"amd64", true}, {"arm64", false}, {"386", true}} {
		t.Run(tc.arch, func(t *testing.T) {
			cfg := winResConfig(t, tc.icon)
			os.WriteFile(cfg.Path("go.mod"), []byte("module hello\n\ngo 1.22\n"), 0o644)
			os.WriteFile(cfg.Path("main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
			exe := filepath.Join(t.TempDir(), "HelloApp.exe")
			tg := target{"windows", tc.arch}
			err := withWindowsResources(cfg, tc.arch, func() error {
				if _, err := os.Stat(cfg.Path(sysoName(tc.arch))); err != nil {
					t.Errorf("resource object missing during the build: %v", err)
				}
				return goBuild(context.Background(), cfg, tg, exe, false)
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(cfg.Path(sysoName(tc.arch))); err == nil {
				t.Error("resource object left in the package")
			}
			f, err := pe.Open(exe)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			s := f.Section(".rsrc")
			if s == nil {
				t.Fatal("no .rsrc section in the executable")
			}
			var dirVA, dirSize uint32
			switch oh := f.OptionalHeader.(type) {
			case *pe.OptionalHeader64:
				dirVA, dirSize = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE].VirtualAddress, oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE].Size
			case *pe.OptionalHeader32:
				dirVA, dirSize = oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE].VirtualAddress, oh.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE].Size
			}
			if dirVA != s.VirtualAddress || dirSize == 0 {
				t.Errorf("resource directory %#x+%d, .rsrc at %#x", dirVA, dirSize, s.VirtualAddress)
			}
			data, err := s.Data()
			if err != nil {
				t.Fatal(err)
			}
			res, _ := walkRsrc(t, data, s.VirtualAddress)
			icons := 0
			if tc.icon {
				icons = len(winIconSizes)
			}
			checkResources(t, res, cfg, icons)
		})
	}
}

func TestWindowsOwnSyso(t *testing.T) {
	cfg := winResConfig(t, false)
	os.WriteFile(cfg.Path("rsrc_windows_amd64.syso"), []byte("theirs"), 0o644)
	ran := false
	err := withWindowsResources(cfg, "amd64", func() error {
		ran = true
		if _, err := os.Stat(cfg.Path(sysoName("amd64"))); err == nil {
			t.Error("tan wrote its object beside the project's own")
		}
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("build ran %v, %v", ran, err)
	}
	// Another architecture's object does not count; a failed build
	// still removes tan's.
	err = withWindowsResources(cfg, "arm64", func() error {
		if _, err := os.Stat(cfg.Path(sysoName("arm64"))); err != nil {
			t.Error(err)
		}
		return os.ErrInvalid
	})
	if err != os.ErrInvalid {
		t.Errorf("build error %v", err)
	}
	if _, err := os.Stat(cfg.Path(sysoName("arm64"))); err == nil {
		t.Error("object left after a failed build")
	}
	if !strings.HasPrefix(sysoName("amd64"), "zz_tan_windows_") {
		t.Error(sysoName("amd64"))
	}
}
