package main

// Windows resources. Before go build for Windows, tan writes a COFF object
// file (.syso) into the main package; the Go linker merges its .rsrc
// section into the executable. The object follows the Microsoft PE/COFF
// specification ("The .rsrc Section") and the Win32 resource formats
// (GRPICONDIR, VS_VERSIONINFO, application manifests). It holds:
//
//	RT_ICON 1…n     the icon frames, PNG compressed (Vista and later)
//	RT_GROUP_ICON 1 the icon directory; id 1 is what the window backend
//	                loads with LoadIconW(instance, 1) and what Explorer shows
//	RT_VERSION 1    VS_VERSIONINFO from tan.json name, version, copyright
//	RT_MANIFEST 1   DPI awareness, common controls 6, UTF-8 code page,
//	                long paths, Windows 10/11 compatibility
//
// Object layout: file header, one section header, the section bytes, its
// relocations, a one-entry symbol table (the section itself) and an empty
// string table. Inside the section: the directory tree (type, then id,
// then language), the data entries, then the resource bytes. A data entry
// holds the RVA of its bytes, so each one gets a relocation of the
// "image-relative address" kind against the section symbol, and the linker
// adds the section's final RVA.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"html"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Win32 resource type ids.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24
)

// winResLang is the language of every resource: English (United States).
// The loader falls back to any language when the user's is missing.
const winResLang = 0x0409

// winResource is one resource to link.
type winResource struct {
	typ, id uint16
	data    []byte
}

// winIconSizes are the frames tan renders for the Windows icon: the
// shell sizes at 100–250 % scaling, and 256 for large views.
var winIconSizes = []int{16, 20, 24, 32, 40, 48, 64, 256}

// icoFrame is one image of an icon: the ICONDIRENTRY fields and the image
// bytes (a PNG, or a BMP without its file header), which are also exactly
// what an RT_ICON resource holds.
type icoFrame struct {
	width, height uint8 // 0 means 256
	colors        uint8 // palette size, 0 when there is none
	planes, bits  uint16
	data          []byte
}

// parseICO splits an .ico file into its frames.
func parseICO(b []byte) ([]icoFrame, error) {
	le := binary.LittleEndian
	if len(b) < 6 || le.Uint16(b) != 0 || le.Uint16(b[2:]) != 1 {
		return nil, errors.New("not an .ico file")
	}
	n := int(le.Uint16(b[4:]))
	if n == 0 || len(b) < 6+16*n {
		return nil, errors.New(".ico file is empty or truncated")
	}
	frames := make([]icoFrame, n)
	for i := range frames {
		e := b[6+16*i:]
		size, off := le.Uint32(e[8:]), le.Uint32(e[12:])
		if uint64(off)+uint64(size) > uint64(len(b)) || size == 0 {
			return nil, fmt.Errorf(".ico image %d lies outside the file", i+1)
		}
		frames[i] = icoFrame{
			width: e[0], height: e[1], colors: e[2],
			planes: le.Uint16(e[4:]), bits: le.Uint16(e[6:]),
			data: b[off : off+size],
		}
	}
	return frames, nil
}

// icoFile encodes frames as an .ico file (ICONDIR, the 16-byte entries
// with file offsets, then the images): the NSIS installer icon
// (package_nsis.go) and the tests of parseICO.
func icoFile(frames []icoFrame) []byte {
	le := binary.LittleEndian
	b := le.AppendUint16(nil, 0)
	b = le.AppendUint16(b, 1)
	b = le.AppendUint16(b, uint16(len(frames)))
	off := uint32(6 + 16*len(frames))
	for _, f := range frames {
		b = append(b, f.width, f.height, f.colors, 0)
		b = le.AppendUint16(b, f.planes)
		b = le.AppendUint16(b, f.bits)
		b = le.AppendUint32(b, uint32(len(f.data)))
		b = le.AppendUint32(b, off)
		off += uint32(len(f.data))
	}
	for _, f := range frames {
		b = append(b, f.data...)
	}
	return b
}

// pngFrames renders img at winIconSizes as PNG frames.
func pngFrames(img image.Image) ([]icoFrame, error) {
	var frames []icoFrame
	for _, n := range winIconSizes {
		data, err := pngBytes(scaleIcon(img, n))
		if err != nil {
			return nil, err
		}
		frames = append(frames, icoFrame{width: uint8(n), height: uint8(n), planes: 1, bits: 32, data: data})
	}
	return frames, nil
}

// windowsIconFrames returns the Windows icon: the frames of "icons.windows"
// when it is an .ico, else frames rendered from "icons.windows" (a PNG
// already in its shape) or from the shape derived from the master icon.
// Nil when the project has no icon.
func windowsIconFrames(cfg *Config) ([]icoFrame, error) {
	if p := cfg.Icons.Windows; strings.EqualFold(filepath.Ext(p), ".ico") {
		b, err := os.ReadFile(cfg.Path(p))
		if err != nil {
			return nil, fmt.Errorf(`"icons.windows": %w`, err)
		}
		frames, err := parseICO(b)
		if err != nil {
			return nil, fmt.Errorf(`"icons.windows" %s: %w`, p, err)
		}
		return frames, nil
	}
	img, err := desktopIconImage(cfg, "windows", cfg.Icons.Windows)
	if err != nil || img == nil {
		return nil, err
	}
	return pngFrames(img)
}

// iconResources turns frames into RT_ICON resources 1…n and the
// RT_GROUP_ICON 1 that lists them (GRPICONDIR: the ICONDIR header, then
// one 14-byte entry per frame that ends in the RT_ICON id instead of a
// file offset).
func iconResources(frames []icoFrame) []winResource {
	if len(frames) == 0 {
		return nil
	}
	le := binary.LittleEndian
	group := le.AppendUint16(nil, 0)                    // reserved
	group = le.AppendUint16(group, 1)                   // type: icon
	group = le.AppendUint16(group, uint16(len(frames))) // count
	var res []winResource
	for i, f := range frames {
		id := uint16(i + 1)
		res = append(res, winResource{typ: rtIcon, id: id, data: f.data})
		group = append(group, f.width, f.height, f.colors, 0)
		group = le.AppendUint16(group, f.planes)
		group = le.AppendUint16(group, f.bits)
		group = le.AppendUint32(group, uint32(len(f.data)))
		group = le.AppendUint16(group, id)
	}
	return append(res, winResource{typ: rtGroupIcon, id: 1, data: group})
}

// winArch maps GOARCH to the COFF machine, the relocation type that
// stores an image-relative (RVA) address, and the manifest's
// processorArchitecture.
func winArch(arch string) (machine, relocRVA uint16, manifestArch string, err error) {
	switch arch {
	case "amd64":
		return 0x8664, 0x0003, "amd64", nil // IMAGE_FILE_MACHINE_AMD64, IMAGE_REL_AMD64_ADDR32NB
	case "arm64":
		return 0xAA64, 0x0002, "arm64", nil // IMAGE_FILE_MACHINE_ARM64, IMAGE_REL_ARM64_ADDR32NB
	case "386":
		return 0x014C, 0x0007, "x86", nil // IMAGE_FILE_MACHINE_I386, IMAGE_REL_I386_DIR32NB
	}
	return 0, 0, "", fmt.Errorf("no Windows resources for architecture %q", arch)
}

// resourceObject builds the COFF object holding res in a .rsrc section.
func resourceObject(arch string, res []winResource) ([]byte, error) {
	machine, relocType, _, err := winArch(arch)
	if err != nil {
		return nil, err
	}
	sect, relocs := rsrcSection(res)
	le := binary.LittleEndian
	const fileHeader, sectionHeader, relocSize = 20, 40, 10
	dataOff := uint32(fileHeader + sectionHeader)
	relocOff := dataOff + uint32(len(sect))
	symOff := relocOff + uint32(relocSize*len(relocs))
	if len(relocs) > 0xFFFF {
		return nil, errors.New("too many resources")
	}

	var b []byte
	// File header.
	b = le.AppendUint16(b, machine)
	b = le.AppendUint16(b, 1) // sections
	b = le.AppendUint32(b, 0) // time stamp: none, for reproducible builds
	b = le.AppendUint32(b, symOff)
	b = le.AppendUint32(b, 1) // symbols
	b = le.AppendUint16(b, 0) // no optional header in an object file
	b = le.AppendUint16(b, 0) // characteristics
	// Section header.
	b = append(b, ".rsrc\x00\x00\x00"...)
	b = le.AppendUint32(b, 0) // VirtualSize: 0 in objects
	b = le.AppendUint32(b, 0) // VirtualAddress: 0 in objects
	b = le.AppendUint32(b, uint32(len(sect)))
	b = le.AppendUint32(b, dataOff)
	b = le.AppendUint32(b, relocOff)
	b = le.AppendUint32(b, 0) // line numbers: none
	b = le.AppendUint16(b, uint16(len(relocs)))
	b = le.AppendUint16(b, 0)
	b = le.AppendUint32(b, 0x00000040|0x40000000) // initialized data, readable
	b = append(b, sect...)
	// Relocations: the field's offset in the section, symbol 0, type.
	for _, off := range relocs {
		b = le.AppendUint32(b, off)
		b = le.AppendUint32(b, 0)
		b = le.AppendUint16(b, relocType)
	}
	// Symbol 0: the section, a static symbol at offset 0 of section 1.
	b = append(b, ".rsrc\x00\x00\x00"...)
	b = le.AppendUint32(b, 0) // value
	b = le.AppendUint16(b, 1) // section number (1-based)
	b = le.AppendUint16(b, 0) // type: not a function
	b = append(b, 3, 0)       // storage class static, no auxiliary records
	// String table: just its own 4-byte size.
	b = le.AppendUint32(b, 4)
	return b, nil
}

// rsrcSection lays out the resource tree and data. It returns the section
// bytes and the offsets of the fields that need an RVA relocation.
//
// Every directory table is 16 bytes (characteristics, time stamp, major
// and minor version, number of named entries, number of id entries)
// followed by 8-byte entries (id, then the offset of a data entry or, with
// the high bit set, of a subdirectory), ids ascending. Offsets are from
// the start of the section.
func rsrcSection(res []winResource) ([]byte, []uint32) {
	res = slices.Clone(res)
	slices.SortFunc(res, func(a, b winResource) int {
		if a.typ != b.typ {
			return int(a.typ) - int(b.typ)
		}
		return int(a.id) - int(b.id)
	})
	var types []uint16
	for _, r := range res {
		if len(types) == 0 || types[len(types)-1] != r.typ {
			types = append(types, r.typ)
		}
	}
	const dirHeader, dirEntry, dataEntry = 16, 8, 16
	const subdir = 0x80000000
	// Offsets: root, the type directories, the id directories (one per
	// resource, each with its single language), the data entries, data.
	typeDirOff := make(map[uint16]uint32)
	off := uint32(dirHeader + dirEntry*len(types))
	for _, t := range types {
		typeDirOff[t] = off
		n := 0
		for _, r := range res {
			if r.typ == t {
				n++
			}
		}
		off += uint32(dirHeader + dirEntry*n)
	}
	idDirOff := make([]uint32, len(res))
	for i := range res {
		idDirOff[i] = off
		off += dirHeader + dirEntry
	}
	entryOff := make([]uint32, len(res))
	for i := range res {
		entryOff[i] = off
		off += dataEntry
	}
	dataOff := make([]uint32, len(res))
	for i, r := range res {
		off = (off + 7) &^ 7
		dataOff[i] = off
		off += uint32(len(r.data))
	}
	size := (off + 3) &^ 3

	le := binary.LittleEndian
	b := make([]byte, size)
	dir := func(at uint32, entries [][2]uint32) {
		le.PutUint16(b[at+14:], uint16(len(entries))) // all entries are ids
		for i, e := range entries {
			p := at + dirHeader + dirEntry*uint32(i)
			le.PutUint32(b[p:], e[0])
			le.PutUint32(b[p+4:], e[1])
		}
	}
	var root [][2]uint32
	for _, t := range types {
		root = append(root, [2]uint32{uint32(t), typeDirOff[t] | subdir})
		var ids [][2]uint32
		for i, r := range res {
			if r.typ == t {
				ids = append(ids, [2]uint32{uint32(r.id), idDirOff[i] | subdir})
			}
		}
		dir(typeDirOff[t], ids)
	}
	dir(0, root)
	relocs := make([]uint32, len(res))
	for i, r := range res {
		dir(idDirOff[i], [][2]uint32{{winResLang, entryOff[i]}})
		// Data entry: RVA of the data (section offset here, relocated),
		// size, code page, reserved.
		le.PutUint32(b[entryOff[i]:], dataOff[i])
		le.PutUint32(b[entryOff[i]+4:], uint32(len(r.data)))
		relocs[i] = entryOff[i]
		copy(b[dataOff[i]:], r.data)
	}
	return b, relocs
}

// winVersion turns "1.2.3" (or "1.2.3-beta+5") into the four 16-bit
// parts of a Windows file version; missing parts are 0.
func winVersion(v string) [4]uint16 {
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	var out [4]uint16
	for i, part := range strings.SplitN(strings.TrimPrefix(v, "v"), ".", 4) {
		if n, err := strconv.ParseUint(part, 10, 64); err == nil {
			out[i] = uint16(min(n, 0xFFFF))
		}
	}
	return out
}

func winVersionString(v [4]uint16) string {
	return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
}

// winManifest renders the application manifest. assemblyIdentity names
// must be letters, digits, '.', '-' or '_', or Windows refuses to start the
// program ("side-by-side configuration is incorrect"), so the id is
// filtered.
func winManifest(cfg *Config, arch string) []byte {
	_, _, procArch, _ := winArch(arch)
	name := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			return r
		}
		return -1
	}, cfg.ID)
	if name == "" {
		name = "app"
	}
	return fmt.Appendf(nil, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="%s" version="%s" processorArchitecture="%s"/>
  <description>%s</description>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <!-- Windows 10 and Windows 11 -->
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2, PerMonitor</dpiAwareness>
      <longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>
      <activeCodePage xmlns="http://schemas.microsoft.com/SMI/2019/WindowsSettings">UTF-8</activeCodePage>
    </windowsSettings>
  </application>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
</assembly>
`, name, winVersionString(winVersion(cfg.Version)), procArch, html.EscapeString(cfg.Name))
}

// verBlock is one node of VS_VERSIONINFO. Every node is laid out the same
// way: wLength (bytes, the node with its children), wValueLength, wType
// (1 text, 0 binary), the key as NUL-terminated UTF-16, padding to 32
// bits, the value, then the children, each starting on a 32-bit boundary.
// wValueLength counts UTF-16 units (with the NUL) for text, bytes for
// binary values.
type verBlock struct {
	key      string
	text     string // a text value, or
	binary   []byte // a binary value
	isText   bool
	children []verBlock
}

func (n verBlock) appendTo(b []byte) []byte {
	le := binary.LittleEndian
	start := len(b)
	var value []byte
	valueLen := len(n.binary)
	if n.isText {
		if n.text != "" { // containers (StringFileInfo, tables) have no value
			value = appendUTF16Z(nil, n.text)
			valueLen = len(value) / 2
		}
	} else {
		value = n.binary
	}
	b = le.AppendUint16(b, 0) // wLength, set below
	b = le.AppendUint16(b, uint16(valueLen))
	if n.isText {
		b = le.AppendUint16(b, 1)
	} else {
		b = le.AppendUint16(b, 0)
	}
	b = appendUTF16Z(b, n.key)
	b = padTo4(b, start)
	b = append(b, value...)
	for _, c := range n.children {
		b = padTo4(b, start)
		b = c.appendTo(b)
	}
	le.PutUint16(b[start:], uint16(len(b)-start))
	return b
}

func appendUTF16Z(b []byte, s string) []byte {
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return append(b, 0, 0)
}

// padTo4 pads b with zeros to a multiple of 4 bytes from start (blocks
// start on 32-bit boundaries, so this aligns within the resource).
func padTo4(b []byte, start int) []byte {
	for (len(b)-start)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

// winVersionInfo renders the RT_VERSION resource: VS_FIXEDFILEINFO, then
// a StringFileInfo with one table for U.S. English in UTF-16 (code page
// 1200, key "040904B0") and the VarFileInfo Translation that points to it.
func winVersionInfo(cfg *Config) []byte {
	v := winVersion(cfg.Version)
	ms, ls := uint32(v[0])<<16|uint32(v[1]), uint32(v[2])<<16|uint32(v[3])
	le := binary.LittleEndian
	var fixed []byte
	for _, d := range []uint32{
		0xFEEF04BD, // signature
		0x00010000, // structure version 1.0
		ms, ls,     // file version
		ms, ls, // product version
		0x3F,       // VS_FFI_FILEFLAGSMASK
		0,          // flags: a release build
		0x00040004, // VOS_NT_WINDOWS32
		0x00000001, // VFT_APP
		0,          // subtype
		0, 0,       // date: none
	} {
		fixed = le.AppendUint32(fixed, d)
	}
	exe := cfg.Exe() + ".exe"
	version := winVersionString(v)
	var strs []verBlock
	for _, kv := range [][2]string{
		{"CompanyName", ""},
		{"FileDescription", cfg.Name},
		{"FileVersion", version},
		{"InternalName", cfg.Exe()},
		{"LegalCopyright", cfg.Copyright},
		{"OriginalFilename", exe},
		{"ProductName", cfg.Name},
		{"ProductVersion", cfg.Version},
	} {
		if kv[1] != "" {
			strs = append(strs, verBlock{key: kv[0], text: kv[1], isText: true})
		}
	}
	translation := le.AppendUint16(nil, winResLang)
	translation = le.AppendUint16(translation, 1200)
	root := verBlock{
		key:    "VS_VERSION_INFO",
		binary: fixed,
		children: []verBlock{
			{key: "StringFileInfo", isText: true, children: []verBlock{
				{key: fmt.Sprintf("%04X%04X", winResLang, 1200), isText: true, children: strs},
			}},
			{key: "VarFileInfo", isText: true, children: []verBlock{
				{key: "Translation", binary: translation},
			}},
		},
	}
	return root.appendTo(nil)
}

// windowsResources gathers what tan links into a Windows executable. An
// icon that cannot be read is a warning; the rest is still linked.
func windowsResources(cfg *Config, arch string) []winResource {
	res := []winResource{
		{typ: rtManifest, id: 1, data: winManifest(cfg, arch)},
		{typ: rtVersion, id: 1, data: winVersionInfo(cfg)},
	}
	frames, err := windowsIconFrames(cfg)
	if err != nil {
		warnf("Windows icon skipped: %v", err)
	}
	return append(res, iconResources(frames)...)
}

// sysoName is the resource object's file name in the main package; the
// _windows_<arch> suffix keeps it out of every other build.
func sysoName(arch string) string { return "zz_tan_windows_" + arch + ".syso" }

// withWindowsResources runs build with the resource object for arch
// written into the main package directory, and removes the object
// afterwards, whether build succeeds or not. When the package already has
// a .syso of its own for this target, tan leaves resources to it.
func withWindowsResources(cfg *Config, arch string, build func() error) error {
	if own := ownSyso(cfg.dir, arch); own != "" {
		warnf("%s is already in the main package; tan adds no icon, manifest or version information", own)
		return build()
	}
	obj, err := resourceObject(arch, windowsResources(cfg, arch))
	if err != nil {
		return err
	}
	path := filepath.Join(cfg.dir, sysoName(arch))
	if err := os.WriteFile(path, obj, 0o644); err != nil {
		return err
	}
	defer os.Remove(path)
	return build()
}

// ownSyso returns the name of a .syso in dir, other than tan's own, that
// go build would link for windows/arch, or "".
func ownSyso(dir, arch string) string {
	names, _ := filepath.Glob(filepath.Join(dir, "*.syso"))
	for _, p := range names {
		name := filepath.Base(p)
		if strings.HasPrefix(name, "zz_tan_windows_") {
			continue
		}
		if sysoMatches(name, "windows", arch) {
			return name
		}
	}
	return ""
}

// sysoMatches applies go build's file name rules (*_GOOS, *_GOARCH,
// *_GOOS_GOARCH; names starting with _ or . are ignored) to a .syso name.
func sysoMatches(name, goos, goarch string) bool {
	if strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".") {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(name, ".syso"), "_")
	n := len(parts)
	if n >= 3 && knownOS[parts[n-2]] && knownArch[parts[n-1]] {
		return parts[n-2] == goos && parts[n-1] == goarch
	}
	if n >= 2 && knownArch[parts[n-1]] {
		return parts[n-1] == goarch
	}
	if n >= 2 && knownOS[parts[n-1]] {
		return parts[n-1] == goos
	}
	return true
}

var knownOS = setOf("aix", "android", "darwin", "dragonfly", "freebsd", "hurd", "illumos", "ios", "js",
	"linux", "nacl", "netbsd", "openbsd", "plan9", "solaris", "wasip1", "windows", "zos")

var knownArch = setOf("386", "amd64", "amd64p32", "arm", "armbe", "arm64", "arm64be", "loong64",
	"mips", "mipsle", "mips64", "mips64le", "mips64p32", "mips64p32le", "ppc", "ppc64", "ppc64le",
	"riscv", "riscv64", "s390", "s390x", "sparc", "sparc64", "wasm")

func setOf(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
