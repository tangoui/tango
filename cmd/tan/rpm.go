package main

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// RPM packages, written without rpmbuild, in the RPM v4 package file format
// (rpm.org "RPM Package format" and "RPM Tags" documents):
//
//	lead (96 bytes) | signature header | padding to 8 | header | payload
//
// Both headers are the same structure: an 8-byte magic, the index entry
// count and the data store size (big-endian), the index entries (tag,
// type, offset, count: 16 bytes each) and the data store. Each header opens
// with a region tag (HEADERSIGNATURES / HEADERIMMUTABLE) whose 16-byte
// trailer ends the data store. The signature header carries the sizes and
// digests of what follows; the payload is a gzip-compressed cpio archive
// in the SVR4 "newc" format, file names as "./usr/...".
//
// The package is unsigned, as rpmbuild's output is before rpmsign; installs
// check its digests.

// rpm header data types.
const (
	rpmTypeInt16       = 3
	rpmTypeInt32       = 4
	rpmTypeString      = 6
	rpmTypeBin         = 7
	rpmTypeStringArray = 8
	rpmTypeI18NString  = 9
)

// rpm tags used here.
const (
	rpmTagHeaderSignatures = 62
	rpmTagHeaderImmutable  = 63
	rpmTagI18NTable        = 100

	rpmSigTagSHA1        = 269
	rpmSigTagSHA256      = 273
	rpmSigTagSize        = 1000
	rpmSigTagMD5         = 1004
	rpmSigTagPayloadSize = 1007

	rpmTagName              = 1000
	rpmTagVersion           = 1001
	rpmTagRelease           = 1002
	rpmTagSummary           = 1004
	rpmTagDescription       = 1005
	rpmTagBuildTime         = 1006
	rpmTagSize              = 1009
	rpmTagLicense           = 1014
	rpmTagPackager          = 1015
	rpmTagGroup             = 1016
	rpmTagURL               = 1020
	rpmTagOS                = 1021
	rpmTagArch              = 1022
	rpmTagPostIn            = 1024
	rpmTagPostUn            = 1026
	rpmTagFileSizes         = 1028
	rpmTagFileModes         = 1030
	rpmTagFileRdevs         = 1033
	rpmTagFileMtimes        = 1034
	rpmTagFileDigests       = 1035
	rpmTagFileLinkTos       = 1036
	rpmTagFileFlags         = 1037
	rpmTagFileUserName      = 1039
	rpmTagFileGroupName     = 1040
	rpmTagSourceRPM         = 1044
	rpmTagFileVerifyFlags   = 1045
	rpmTagProvideName       = 1047
	rpmTagRequireFlags      = 1048
	rpmTagRequireName       = 1049
	rpmTagRequireVersion    = 1050
	rpmTagPostInProg        = 1086
	rpmTagPostUnProg        = 1088
	rpmTagFileDevices       = 1095
	rpmTagFileInodes        = 1096
	rpmTagFileLangs         = 1097
	rpmTagProvideFlags      = 1112
	rpmTagProvideVersion    = 1113
	rpmTagDirIndexes        = 1116
	rpmTagBaseNames         = 1117
	rpmTagDirNames          = 1118
	rpmTagPayloadFormat     = 1124
	rpmTagPayloadCompressor = 1125
	rpmTagPayloadFlags      = 1126
	rpmTagFileDigestAlgo    = 5011
	rpmTagEncoding          = 5062
	rpmTagPayloadDigest     = 5092
	rpmTagPayloadDigestAlgo = 5093
)

// Dependency flags (rpmds.h, RPMSENSE_*).
const (
	rpmSenseLess         = 1 << 1
	rpmSenseGreater      = 1 << 2
	rpmSenseEqual        = 1 << 3
	rpmSenseInterp       = 1 << 8
	rpmSenseScriptPost   = 1 << 10
	rpmSenseScriptPostUn = 1 << 12
	rpmSenseRPMLib       = 1 << 24
)

const rpmDigestSHA256 = 8 // PGPHASHALGO_SHA256

var (
	rpmLeadMagic   = []byte{0xed, 0xab, 0xee, 0xdb}
	rpmHeaderMagic = []byte{0x8e, 0xad, 0xe8, 0x01, 0, 0, 0, 0}
)

// rpmArch maps GOARCH to the rpm architecture.
func rpmArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armv7hl"
	}
	return goarch // riscv64, ppc64le, s390x
}

type rpmEntry struct {
	tag, typ uint32
	count    int
	data     []byte
}

// rpmHeader collects index entries; marshal lays them out.
type rpmHeader []rpmEntry

func (h *rpmHeader) add(tag, typ uint32, count int, data []byte) {
	*h = append(*h, rpmEntry{tag, typ, count, data})
}

// rpmCString is s NUL-terminated, with any NUL inside it dropped: header
// strings end at the first NUL, so one inside would shift every later
// string of the entry.
func rpmCString(s string) []byte { return append([]byte(strings.ReplaceAll(s, "\x00", "")), 0) }

func (h *rpmHeader) str(tag uint32, s string) { h.add(tag, rpmTypeString, 1, rpmCString(s)) }

// i18n is a translatable string with only the "C" locale (I18NTABLE).
func (h *rpmHeader) i18n(tag uint32, s string) { h.add(tag, rpmTypeI18NString, 1, rpmCString(s)) }

func (h *rpmHeader) strs(tag uint32, ss []string) {
	var b []byte
	for _, s := range ss {
		b = append(b, rpmCString(s)...)
	}
	h.add(tag, rpmTypeStringArray, len(ss), b)
}

func (h *rpmHeader) int32s(tag uint32, vs ...uint32) {
	var b []byte
	for _, v := range vs {
		b = binary.BigEndian.AppendUint32(b, v)
	}
	h.add(tag, rpmTypeInt32, len(vs), b)
}

func (h *rpmHeader) int16s(tag uint32, vs ...uint16) {
	var b []byte
	for _, v := range vs {
		b = binary.BigEndian.AppendUint16(b, v)
	}
	h.add(tag, rpmTypeInt16, len(vs), b)
}

// marshal encodes the header with region tag region. Index entries are
// sorted by tag, after the region entry, and their data laid out in the
// same order, each aligned to its type's size; the region trailer, last in
// the store, points back over the whole index (offset -16·entries).
func (h rpmHeader) marshal(region uint32) []byte {
	es := slices.Clone(h)
	slices.SortStableFunc(es, func(a, b rpmEntry) int { return cmp.Compare(a.tag, b.tag) })
	n := len(es) + 1
	var index, store []byte
	entry := func(b []byte, tag, typ, off uint32, count int) []byte {
		b = binary.BigEndian.AppendUint32(b, tag)
		b = binary.BigEndian.AppendUint32(b, typ)
		b = binary.BigEndian.AppendUint32(b, off)
		return binary.BigEndian.AppendUint32(b, uint32(count))
	}
	for _, e := range es {
		align := map[uint32]int{rpmTypeInt16: 2, rpmTypeInt32: 4}[e.typ]
		for align > 0 && len(store)%align != 0 {
			store = append(store, 0)
		}
		index = entry(index, e.tag, e.typ, uint32(len(store)), e.count)
		store = append(store, e.data...)
	}
	trailerAt := uint32(len(store))
	store = entry(store, region, rpmTypeBin, uint32(-int32(n*16)), 16)
	index = append(entry(nil, region, rpmTypeBin, trailerAt, 16), index...)

	out := slices.Clone(rpmHeaderMagic)
	out = binary.BigEndian.AppendUint32(out, uint32(n))
	out = binary.BigEndian.AppendUint32(out, uint32(len(store)))
	out = append(out, index...)
	return append(out, store...)
}

// rpmLead is the 96-byte lead: magic, format 3.0, binary type, arch number,
// "name-version-release", OS linux (1), signature type 5 (a header).
func rpmLead(nvr, arch string) []byte {
	b := slices.Clone(rpmLeadMagic)
	b = append(b, 3, 0)
	b = binary.BigEndian.AppendUint16(b, 0) // binary package
	archNum := map[string]uint16{"x86_64": 1, "i686": 1, "aarch64": 19}[arch]
	b = binary.BigEndian.AppendUint16(b, archNum)
	name := make([]byte, 66)
	copy(name[:65], nvr)
	b = append(b, name...)
	b = binary.BigEndian.AppendUint16(b, 1) // Linux
	b = binary.BigEndian.AppendUint16(b, 5) // RPMSIGTYPE_HEADERSIG
	return append(b, make([]byte, 16)...)
}

// cpioNewc writes files as an SVR4 "newc" (070701) archive: per file a
// 110-byte header of hexadecimal fields, the NUL-terminated name, padding
// to 4 bytes, the data, padding to 4; then the TRAILER!!! entry. Inode i+1
// matches FILEINODES.
func cpioNewc(files []sysFile, mtime time.Time) []byte {
	var b bytes.Buffer
	pad := func() {
		for b.Len()%4 != 0 {
			b.WriteByte(0)
		}
	}
	entry := func(ino int, mode int64, size int, name string) {
		fmt.Fprintf(&b, "070701%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
			ino, mode, 0, 0, 1, mtime.Unix(), size, 0, 0, 0, 0, len(name)+1, 0)
		b.WriteString(name)
		b.WriteByte(0)
		pad()
	}
	for i, f := range files {
		entry(i+1, 0o100000|f.mode, len(f.data), "."+f.path)
		b.Write(f.data)
		pad()
	}
	entry(0, 0, 0, "TRAILER!!!")
	return b.Bytes()
}

// rpmRequire parses a "linux.rpmRequires" entry: "name" or "name op
// version" with op one of < <= = >= >.
func rpmRequire(s string) (name string, flags uint32, version string) {
	f := strings.Fields(s)
	if len(f) != 3 {
		return strings.TrimSpace(s), 0, ""
	}
	ops := map[string]uint32{"<": rpmSenseLess, "<=": rpmSenseLess | rpmSenseEqual, "=": rpmSenseEqual,
		"==": rpmSenseEqual, ">=": rpmSenseGreater | rpmSenseEqual, ">": rpmSenseGreater}
	if op, ok := ops[f[1]]; ok {
		return f[0], op, f[2]
	}
	return strings.TrimSpace(s), 0, ""
}

// rpmDefaultRequires are the sonames of GTK 3 and WebKitGTK 4.1, as the
// libraries' packages provide them on every rpm distribution (Fedora's
// gtk3 and webkit2gtk4.1, openSUSE's libgtk-3-0 and libwebkit2gtk-4_1-0).
func rpmDefaultRequires(goarch string) []string {
	suffix := "()(64bit)"
	if goarch == "386" || goarch == "arm" {
		suffix = "()"
	}
	return []string{"libc.so.6" + suffix, "libgtk-3.so.0" + suffix, "libwebkit2gtk-4.1.so.0" + suffix}
}

// packageRPM writes <base>.rpm next to the build directory.
func packageRPM(cfg *Config, a artifact, icons map[string][]byte) (string, error) {
	files, err := linuxSystemFiles(cfg, a, icons)
	if err != nil {
		return "", err
	}
	mtime := packageTime()
	name, version, release := linuxPackageName(cfg), linuxPackageVersion(cfg.Version), "1"
	arch := rpmArch(a.target.Arch)
	script := linuxCacheScript(files)

	cpio := cpioNewc(files, mtime)
	var gz bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
	zw.Write(cpio)
	if err := zw.Close(); err != nil {
		return "", err
	}
	payload := gz.Bytes()

	var h rpmHeader
	h.strs(rpmTagI18NTable, []string{"C"})
	h.str(rpmTagName, name)
	h.str(rpmTagVersion, version)
	h.str(rpmTagRelease, release)
	h.i18n(rpmTagSummary, oneLine(linuxSummary(cfg)))
	h.i18n(rpmTagDescription, cmp.Or(linuxDescription(cfg), linuxSummary(cfg)))
	h.int32s(rpmTagBuildTime, uint32(mtime.Unix()))
	h.str(rpmTagLicense, cmp.Or(cfg.Linux.License, "Proprietary"))
	if cfg.Linux.Maintainer != "" {
		h.str(rpmTagPackager, cfg.Linux.Maintainer)
	}
	h.i18n(rpmTagGroup, "Unspecified")
	if cfg.Linux.Homepage != "" {
		h.str(rpmTagURL, cfg.Linux.Homepage)
	}
	h.str(rpmTagOS, "linux")
	h.str(rpmTagArch, arch)
	h.str(rpmTagSourceRPM, fmt.Sprintf("%s-%s-%s.src.rpm", name, version, release))
	h.str(rpmTagPostIn, script)
	h.strs(rpmTagPostInProg, []string{"/bin/sh"})
	h.str(rpmTagPostUn, script)
	h.strs(rpmTagPostUnProg, []string{"/bin/sh"})

	// The file list: parallel arrays, paths split into directory names
	// (with a trailing '/') and base names.
	var (
		size                                     uint32
		sizes, mtimes, flags, verify, devs, inos []uint32
		dirIndexes                               []uint32
		modes, rdevs                             []uint16
		digests, links, users, groups, langs     []string
		dirs, bases                              []string
	)
	for i, f := range files {
		size += uint32(len(f.data))
		sizes = append(sizes, uint32(len(f.data)))
		mtimes = append(mtimes, uint32(mtime.Unix()))
		flags = append(flags, 0)
		verify = append(verify, 0xffffffff)
		devs = append(devs, 1)
		inos = append(inos, uint32(i+1))
		modes = append(modes, uint16(0o100000|f.mode))
		rdevs = append(rdevs, 0)
		sum := sha256.Sum256(f.data)
		digests = append(digests, hex.EncodeToString(sum[:]))
		links = append(links, "")
		users = append(users, "root")
		groups = append(groups, "root")
		langs = append(langs, "")
		dir := path.Dir(f.path) + "/"
		k := slices.Index(dirs, dir)
		if k < 0 {
			k = len(dirs)
			dirs = append(dirs, dir)
		}
		dirIndexes = append(dirIndexes, uint32(k))
		bases = append(bases, path.Base(f.path))
	}
	h.int32s(rpmTagSize, size)
	h.int32s(rpmTagFileSizes, sizes...)
	h.int16s(rpmTagFileModes, modes...)
	h.int16s(rpmTagFileRdevs, rdevs...)
	h.int32s(rpmTagFileMtimes, mtimes...)
	h.strs(rpmTagFileDigests, digests)
	h.strs(rpmTagFileLinkTos, links)
	h.int32s(rpmTagFileFlags, flags...)
	h.strs(rpmTagFileUserName, users)
	h.strs(rpmTagFileGroupName, groups)
	h.int32s(rpmTagFileVerifyFlags, verify...)
	h.int32s(rpmTagFileDevices, devs...)
	h.int32s(rpmTagFileInodes, inos...)
	h.strs(rpmTagFileLangs, langs)
	h.int32s(rpmTagDirIndexes, dirIndexes...)
	h.strs(rpmTagBaseNames, bases)
	h.strs(rpmTagDirNames, dirs)
	h.int32s(rpmTagFileDigestAlgo, rpmDigestSHA256)

	// Dependencies: the rpmlib features this layout uses, the scriptlet
	// interpreter, the runtime libraries, then "linux.rpmRequires".
	type dep struct {
		name    string
		flags   uint32
		version string
	}
	reqs := []dep{
		{"rpmlib(CompressedFileNames)", rpmSenseRPMLib | rpmSenseLess | rpmSenseEqual, "3.0.4-1"},
		{"rpmlib(FileDigests)", rpmSenseRPMLib | rpmSenseLess | rpmSenseEqual, "4.6.0-1"},
		{"rpmlib(PayloadFilesHavePrefix)", rpmSenseRPMLib | rpmSenseLess | rpmSenseEqual, "4.0-1"},
		{"/bin/sh", rpmSenseInterp | rpmSenseScriptPost, ""},
		{"/bin/sh", rpmSenseInterp | rpmSenseScriptPostUn, ""},
	}
	for _, r := range append(rpmDefaultRequires(a.target.Arch), cfg.Linux.RPMRequires...) {
		n, f, v := rpmRequire(r)
		reqs = append(reqs, dep{n, f, v})
	}
	var reqNames, reqVersions []string
	var reqFlags []uint32
	for _, r := range reqs {
		reqNames = append(reqNames, r.name)
		reqFlags = append(reqFlags, r.flags)
		reqVersions = append(reqVersions, r.version)
	}
	h.strs(rpmTagRequireName, reqNames)
	h.int32s(rpmTagRequireFlags, reqFlags...)
	h.strs(rpmTagRequireVersion, reqVersions)
	h.strs(rpmTagProvideName, []string{name})
	h.int32s(rpmTagProvideFlags, rpmSenseEqual)
	h.strs(rpmTagProvideVersion, []string{version + "-" + release})

	h.str(rpmTagPayloadFormat, "cpio")
	h.str(rpmTagPayloadCompressor, "gzip")
	h.str(rpmTagPayloadFlags, "9")
	h.str(rpmTagEncoding, "utf-8")
	pd := sha256.Sum256(payload)
	h.strs(rpmTagPayloadDigest, []string{hex.EncodeToString(pd[:])})
	h.int32s(rpmTagPayloadDigestAlgo, rpmDigestSHA256)
	header := h.marshal(rpmTagHeaderImmutable)

	// Signature header: digests of the header (SHA-1, SHA-256) and of
	// header and payload (MD5), and their sizes.
	var s rpmHeader
	sha1Sum := sha1.Sum(header)
	sha256Sum := sha256.Sum256(header)
	md5Sum := md5.New()
	md5Sum.Write(header)
	md5Sum.Write(payload)
	s.str(rpmSigTagSHA1, hex.EncodeToString(sha1Sum[:]))
	s.str(rpmSigTagSHA256, hex.EncodeToString(sha256Sum[:]))
	s.int32s(rpmSigTagSize, uint32(len(header)+len(payload)))
	s.add(rpmSigTagMD5, rpmTypeBin, 16, md5Sum.Sum(nil))
	s.int32s(rpmSigTagPayloadSize, uint32(len(cpio)))
	sig := s.marshal(rpmTagHeaderSignatures)
	for len(sig)%8 != 0 {
		sig = append(sig, 0)
	}

	out := filepath.Join(filepath.Dir(a.dir), artifactBase(cfg, a.target)+".rpm")
	var b bytes.Buffer
	b.Write(rpmLead(name+"-"+version+"-"+release, arch))
	b.Write(sig)
	b.Write(header)
	b.Write(payload)
	if err := os.WriteFile(out, b.Bytes(), 0o644); err != nil {
		return "", err
	}
	return out, nil
}
