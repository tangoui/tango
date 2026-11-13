package main

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

// readDSStore parses a .DS_Store the way the format describes, checking
// the allocator bookkeeping, and returns its records in file order.
func readDSStore(t *testing.T, b []byte) []dsRecord {
	t.Helper()
	be := binary.BigEndian
	if len(b) < 36 || be.Uint32(b) != 1 || string(b[4:8]) != "Bud1" {
		t.Fatal("bad header")
	}
	at := func(off uint32) []byte { return b[4+off:] }
	infoOff, infoSize := be.Uint32(b[8:]), be.Uint32(b[12:])
	if be.Uint32(b[16:]) != infoOff || infoSize != 2048 {
		t.Fatalf("header offsets %d %d", infoOff, infoSize)
	}
	info := at(infoOff)
	n := be.Uint32(info)
	var addrs []uint32
	for i := range n {
		addrs = append(addrs, be.Uint32(info[8+4*i:]))
	}
	p := info[8+4*256:]
	dirs := be.Uint32(p)
	p = p[4:]
	blocks := map[string]uint32{}
	for range dirs {
		l := p[0]
		name := string(p[1 : 1+l])
		blocks[name] = be.Uint32(p[1+l:])
		p = p[5+l:]
	}
	// Allocated blocks (the header's 32 bytes and each address) and free
	// blocks must tile the 2^31-byte space exactly, each aligned to its size.
	type span struct{ off, size uint64 }
	spans := []span{{0, 32}}
	for _, a := range addrs {
		spans = append(spans, span{uint64(a &^ 31), 1 << (a & 31)})
	}
	for k := range 32 {
		c := be.Uint32(p)
		p = p[4:]
		for range c {
			spans = append(spans, span{uint64(be.Uint32(p)), 1 << k})
			p = p[4:]
		}
	}
	slices.SortFunc(spans, func(a, b span) int { return int(a.off) - int(b.off) })
	var next uint64
	for _, s := range spans {
		if s.off != next || s.off%s.size != 0 {
			t.Fatalf("allocator: block at %d size %d, expected offset %d", s.off, s.size, next)
		}
		next += s.size
	}
	if next != 1<<31 {
		t.Fatalf("allocator covers %d bytes", next)
	}
	if addrs[0]&^31 != infoOff {
		t.Error("block 0 is not the info block")
	}
	dsdb := at(addrs[blocks["DSDB"]] &^ 31)
	root, levels, count, nodes, page := be.Uint32(dsdb), be.Uint32(dsdb[4:]), be.Uint32(dsdb[8:]), be.Uint32(dsdb[12:]), be.Uint32(dsdb[16:])
	if levels != 0 || nodes != 1 || page != 0x1000 {
		t.Fatalf("DSDB %d %d %d %d", root, levels, nodes, page)
	}
	node := at(addrs[root] &^ 31)
	if be.Uint32(node) != 0 || be.Uint32(node[4:]) != count {
		t.Fatal("node header")
	}
	q := node[8:]
	var recs []dsRecord
	for range count {
		l := be.Uint32(q)
		u := make([]uint16, l)
		for i := range u {
			u[i] = be.Uint16(q[4+2*i:])
		}
		q = q[4+2*l:]
		r := dsRecord{name: string(utf16.Decode(u)), code: string(q[:4]), typ: string(q[4:8])}
		q = q[8:]
		var size int
		switch r.typ {
		case "blob":
			size = 4 + int(be.Uint32(q))
		case "long", "shor", "type":
			size = 4
		case "bool":
			size = 1
		default:
			t.Fatalf("type %q", r.typ)
		}
		r.value = q[:size]
		q = q[size:]
		recs = append(recs, r)
	}
	return recs
}

func TestDSStoreRoundTrip(t *testing.T) {
	l := dmgLayout{title: "Hello", width: 600, height: 400, iconSize: 96, appX: 150, appY: 200, applicationsX: 450, applY: 200}
	recs, err := dmgStoreRecords(l, "Hello.app", []byte("ALIAS"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := encodeDSStore(recs)
	if err != nil {
		t.Fatal(err)
	}
	got := readDSStore(t, b)
	var keys []string
	for _, r := range got {
		keys = append(keys, r.name+"/"+r.code)
	}
	want := []string{"./bwsp", "./icvp", "./vSrn", "./vstl", ".background/Iloc", ".DS_Store/Iloc", ".fseventsd/Iloc", ".VolumeIcon.icns/Iloc", "Applications/Iloc", "Hello.app/Iloc"}
	if !slices.Equal(keys, want) {
		t.Errorf("records\n%v\nwant\n%v", keys, want)
	}
	for _, r := range got {
		if r.name == "Hello.app" && r.code == "Iloc" {
			v := r.value
			if be := binary.BigEndian; be.Uint32(v) != 16 || be.Uint32(v[4:]) != 150 || be.Uint32(v[8:]) != 200 {
				t.Errorf("app Iloc % x", v)
			}
		}
		if r.code == "icvp" && !bytes.Contains(r.value, []byte("ALIAS")) {
			t.Error("icvp lacks the background alias")
		}
	}
	if !bytes.Equal(b, must(encodeDSStore(reversed(recs)))) {
		t.Error("output depends on record order")
	}
}

func reversed[T any](s []T) []T { r := slices.Clone(s); slices.Reverse(r); return r }

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestBplist(t *testing.T) {
	v := map[string]any{
		"WindowBounds": "{{200, 120}, {540, 380}}", "ShowSidebar": false, "on": true,
		"iconSize": 128.5, "small": 7, "big": 70000, "huge": 1 << 40, "data": []byte{1, 2, 3},
		"名前": "日本語", "long": strings.Repeat("x", 40),
	}
	b, err := encodeBplist(v)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("bplist00")) {
		t.Fatal("magic")
	}
	plutil, err := exec.LookPath("plutil")
	if err != nil {
		t.Skip("no plutil to read the plist back")
	}
	f := filepath.Join(t.TempDir(), "x.plist")
	os.WriteFile(f, b, 0o644)
	out, err := exec.Command(plutil, "-convert", "xml1", "-o", "-", f).CombinedOutput()
	if err != nil {
		t.Fatalf("plutil: %v %s", err, out)
	}
	for _, want := range []string{"<string>{{200, 120}, {540, 380}}</string>", "<key>ShowSidebar</key>\n\t<false/>", "<real>128.5</real>",
		"<integer>70000</integer>", "<integer>1099511627776</integer>", "<data>\n\tAQID\n\t</data>", "<key>名前</key>\n\t<string>日本語</string>", strings.Repeat("x", 40)} {
		if !strings.Contains(string(out), want) {
			t.Errorf("plist lacks %q:\n%s", want, out)
		}
	}
}

func TestMacAlias(t *testing.T) {
	vol := time.Date(2026, 11, 13, 8, 0, 0, 0, time.FixedZone("CST", 8*3600))
	a := encodeMacAlias(macAliasTarget{
		volumeName: "Hello", volumeDate: vol, mountPoint: "/Volumes/Hello", path: "/.background/background.png",
		parentID: 18, fileID: 19, fileDate: vol, ejectable: true,
	})
	be := binary.BigEndian
	if int(be.Uint16(a[4:])) != len(a) || be.Uint16(a[6:]) != 2 {
		t.Fatalf("size %d / %d, version %d", be.Uint16(a[4:]), len(a), be.Uint16(a[6:]))
	}
	if a[10] != 5 || string(a[11:16]) != "Hello" || be.Uint32(a[38:]) != uint32(vol.Unix()+8*3600+2082844800) || string(a[42:44]) != "H+" || be.Uint16(a[44:]) != 5 {
		t.Errorf("volume fields % x", a[10:46])
	}
	if be.Uint32(a[46:]) != 18 || a[50] != 14 || string(a[51:65]) != "background.png" || be.Uint32(a[114:]) != 19 {
		t.Errorf("file fields % x", a[46:118])
	}
	// Walk the tags.
	tags := map[int16][]byte{}
	p := a[150:]
	for {
		id, n := int16(be.Uint16(p)), int(be.Uint16(p[2:]))
		if id == -1 {
			if n != 0 || len(p) != 4 {
				t.Errorf("end tag with %d bytes after", len(p))
			}
			break
		}
		tags[id] = p[4 : 4+n]
		p = p[4+n+n%2:]
	}
	if string(tags[0]) != ".background" || string(tags[2]) != "Hello:.background:\x00background.png" || string(tags[18]) != "/.background/background.png" || string(tags[19]) != "/Volumes/Hello" {
		t.Errorf("tags %v", tags)
	}
	if be.Uint64(tags[16]) != uint64(vol.Unix()+2082844800)<<16 || be.Uint64(tags[17]) != be.Uint64(tags[16]) {
		t.Errorf("high-resolution dates % x % x", tags[16], tags[17])
	}
	if be.Uint16(tags[14]) != 14 || be.Uint16(tags[15]) != 5 {
		t.Errorf("unicode names % x / % x", tags[14], tags[15])
	}
}

func TestDMGLayoutDefaults(t *testing.T) {
	cfg := testConfig(t)
	l := dmgLayoutFor(cfg)
	if l != (dmgLayout{title: "Hello App", width: 540, height: 380, iconSize: 128, appX: 140, appY: 190, applicationsX: 400, applY: 190}) {
		t.Errorf("defaults %+v", l)
	}
	cfg.Installer.MacOS.DMG = DMGLayout{Background: "bg.png", WindowSize: []int{600, 420}, IconSize: 100, AppPosition: []int{1, 2}, ApplicationsPosition: []int{3, 4}, Title: "Install"}
	l = dmgLayoutFor(cfg)
	if l.title != "Install" || l.width != 600 || l.height != 420 || l.iconSize != 100 || l.appX != 1 || l.applY != 4 || l.background != cfg.Path("bg.png") {
		t.Errorf("custom %+v", l)
	}
}

// TestPackageDarwinDMG makes a real disk image with hdiutil (macOS only;
// TANGO_DMG_TEST=1, as it mounts images) and reads the layout back.
func TestPackageDarwinDMG(t *testing.T) {
	if runtime.GOOS != "darwin" || os.Getenv("TANGO_DMG_TEST") == "" {
		t.Skip("set TANGO_DMG_TEST=1 on macOS")
	}
	cfg := testConfig(t)
	os.WriteFile(cfg.Path("bg.png"), testPNG(540, 380, color.NRGBA{30, 60, 120, 255}), 0o644)
	cfg.Installer.MacOS.DMG.Background = "bg.png"
	a := artifact{target: target{"darwin", "arm64"}, dir: cfg.Path("build/darwin-arm64")}
	a.path = filepath.Join(a.dir, "Hello App.app")
	os.MkdirAll(filepath.Join(a.path, "Contents", "Resources"), 0o755)
	os.WriteFile(filepath.Join(a.path, "Contents", "Info.plist"), []byte("<plist/>"), 0o644)
	os.WriteFile(filepath.Join(a.path, "Contents", "Resources", "icon.icns"), []byte("icns"), 0o644)
	made, err := packageDarwin(t.Context(), cfg, a)
	if err != nil {
		t.Fatal(err)
	}
	mnt := t.TempDir()
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-noautoopen", "-readonly", "-mountpoint", mnt, made[0]).CombinedOutput(); err != nil {
		t.Fatalf("attach: %v %s", err, out)
	}
	defer exec.Command("hdiutil", "detach", "-quiet", mnt).Run()
	for _, f := range []string{"Hello App.app/Contents/Info.plist", ".background/background.png", ".VolumeIcon.icns", ".DS_Store"} {
		if _, err := os.Stat(filepath.Join(mnt, f)); err != nil {
			t.Error(err)
		}
	}
	if l, _ := os.Readlink(filepath.Join(mnt, "Applications")); l != "/Applications" {
		t.Errorf("Applications link %q", l)
	}
	if _, err := os.Stat(filepath.Join(mnt, ".fseventsd")); err == nil {
		t.Error(".fseventsd left on the volume")
	}
	b, _ := os.ReadFile(filepath.Join(mnt, ".DS_Store"))
	recs := readDSStore(t, b)
	if len(recs) != 10 {
		t.Errorf("%d records", len(recs))
	}
	out, _ := exec.Command("xattr", "-px", "com.apple.FinderInfo", mnt).Output()
	if f := strings.Fields(string(out)); len(f) < 10 || f[8] != "04" {
		t.Errorf("volume FinderInfo %q: no custom icon flag", out)
	}
}
