package main

// Finder's .DS_Store files, written in pure Go so a disk image gets its
// window layout and background without Finder or AppleScript (and so on
// a build machine without a graphical session).
//
// The format, as documented publicly (Mark Mentovai's and Wim L's
// descriptions of the "Bud1" buddy-allocator store used by Finder):
//
//	file    u32 1, "Bud1", then the allocator header: u32 offset and u32
//	        size of the info block, the offset again, 16 unused bytes.
//	        Every offset counts from byte 4 of the file.
//	info    u32 block count, u32 0, the block addresses (offset | log2
//	        size, padded to a multiple of 256 entries), the directory
//	        (u32 count; u8 name length, name, u32 block number), then 32
//	        free lists (u32 count, offsets), one per block size 2^0…2^31.
//	"DSDB"  the B-tree header: root node block, internal levels, record
//	        count, node count, page size 0x1000.
//	node    u32 0 (a leaf), u32 record count, the records sorted by file
//	        name (case-insensitively), then by structure code.
//	record  u32 file name length in UTF-16 units, the name in UTF-16BE,
//	        4-byte structure code ("Iloc", "bwsp", "icvp"…), 4-byte type
//	        ("blob", "long", "type", "bool"…), the value.
//
// The allocator hands out power-of-two blocks aligned to their size from
// a 2^31-byte space; every byte is either in a block or on a free list.
//
// Also here: the binary property lists (bplist00) Finder keeps in the
// "bwsp" and "icvp" records, and the Alias Manager record (version 2)
// that names the background picture.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf16"
)

// dsRecord is one .DS_Store record: a property (code) of a file in the
// folder (name; "." is the folder itself).
type dsRecord struct {
	name, code string
	typ        string // blob, long, shor, bool, type, ustr
	value      []byte // encoded, without the type
}

func dsBlob(name, code string, b []byte) dsRecord {
	v := binary.BigEndian.AppendUint32(nil, uint32(len(b)))
	return dsRecord{name, code, "blob", append(v, b...)}
}

func dsLong(name, code string, v uint32) dsRecord {
	return dsRecord{name, code, "long", binary.BigEndian.AppendUint32(nil, v)}
}

func dsBool(name, code string, v bool) dsRecord {
	b := byte(0)
	if v {
		b = 1
	}
	return dsRecord{name, code, "bool", []byte{b}}
}

func dsType(name, code, v string) dsRecord { return dsRecord{name, code, "type", []byte(v)} }

// dsIloc places an icon: its centre, in points from the top left of the
// window's content.
func dsIloc(name string, x, y int) dsRecord {
	be := binary.BigEndian
	b := be.AppendUint32(nil, uint32(x))
	b = be.AppendUint32(b, uint32(y))
	b = be.AppendUint32(b, 0xFFFFFFFF)
	b = be.AppendUint32(b, 0xFFFF0000)
	return dsBlob(name, "Iloc", b)
}

// dsLess orders records as Finder's B-tree does.
func dsLess(a, b dsRecord) int {
	if c := strings.Compare(strings.ToLower(a.name), strings.ToLower(b.name)); c != 0 {
		return c
	}
	return strings.Compare(a.code, b.code)
}

// dsBuddy is the buddy allocator of the store's address space.
type dsBuddy struct{ free [32][]uint32 }

func newDSBuddy() *dsBuddy {
	b := &dsBuddy{}
	b.free[31] = []uint32{0}
	return b
}

// alloc takes a block of 2^log bytes.
func (b *dsBuddy) alloc(log int) uint32 {
	k := log
	for k < 32 && len(b.free[k]) == 0 {
		k++
	}
	if k == 32 {
		panic("dsstore: allocator full")
	}
	off := b.free[k][0]
	b.free[k] = b.free[k][1:]
	for k > log { // split, keeping the lower half
		k--
		b.free[k] = append(b.free[k], off+1<<k)
		slices.Sort(b.free[k])
	}
	return off
}

// log2Ceil is the smallest k with 2^k ≥ n, at least 5 (32 bytes).
func log2Ceil(n int) int {
	k := 5
	for 1<<k < n {
		k++
	}
	return k
}

// encodeDSStore writes a .DS_Store holding records in a single B-tree
// leaf (plenty for a disk image's handful of entries).
func encodeDSStore(records []dsRecord) ([]byte, error) {
	records = slices.Clone(records)
	slices.SortStableFunc(records, dsLess)
	be := binary.BigEndian

	node := be.AppendUint32(nil, 0) // leaf
	node = be.AppendUint32(node, uint32(len(records)))
	for _, r := range records {
		if len(r.code) != 4 || len(r.typ) != 4 {
			return nil, fmt.Errorf("dsstore: bad record %q %q %q", r.name, r.code, r.typ)
		}
		name := utf16.Encode([]rune(r.name))
		node = be.AppendUint32(node, uint32(len(name)))
		for _, u := range name {
			node = be.AppendUint16(node, u)
		}
		node = append(node, r.code...)
		node = append(node, r.typ...)
		node = append(node, r.value...)
	}
	const pageSize = 0x1000
	if len(node) > pageSize {
		return nil, errors.New("dsstore: records do not fit in one page")
	}

	buddy := newDSBuddy()
	buddy.alloc(5) // the file header's own 32 bytes at offset 0
	infoOff := buddy.alloc(11)
	dsdbOff := buddy.alloc(5)
	nodeOff := buddy.alloc(12)
	addrs := []uint32{infoOff | 11, dsdbOff | 5, nodeOff | 12}

	dsdb := be.AppendUint32(nil, 2) // root node: block 2
	dsdb = be.AppendUint32(dsdb, 0) // internal levels
	dsdb = be.AppendUint32(dsdb, uint32(len(records)))
	dsdb = be.AppendUint32(dsdb, 1) // nodes
	dsdb = be.AppendUint32(dsdb, pageSize)

	info := be.AppendUint32(nil, uint32(len(addrs)))
	info = be.AppendUint32(info, 0)
	for i := range 256 {
		var a uint32
		if i < len(addrs) {
			a = addrs[i]
		}
		info = be.AppendUint32(info, a)
	}
	info = be.AppendUint32(info, 1) // directory entries
	info = append(info, 4)
	info = append(info, "DSDB"...)
	info = be.AppendUint32(info, 1) // block 1
	for _, list := range buddy.free {
		info = be.AppendUint32(info, uint32(len(list)))
		for _, off := range list {
			info = be.AppendUint32(info, off)
		}
	}
	if len(info) > 1<<11 {
		return nil, errors.New("dsstore: info block overflow")
	}

	end := nodeOff + pageSize
	out := make([]byte, 4+end)
	be.PutUint32(out, 1)
	copy(out[4:], "Bud1")
	be.PutUint32(out[8:], infoOff)
	be.PutUint32(out[12:], 1<<11)
	be.PutUint32(out[16:], infoOff)
	copy(out[4+infoOff:], info)
	copy(out[4+dsdbOff:], dsdb)
	copy(out[4+nodeOff:], node)
	return out, nil
}

// plist values for encodeBplist: map[string]any, string, bool, int,
// float64 and []byte.

// encodeBplist writes v as a binary property list (bplist00): the
// objects, the offset table, and the 32-byte trailer.
func encodeBplist(v any) ([]byte, error) {
	var count func(any) int
	count = func(v any) int {
		n := 1
		if m, ok := v.(map[string]any); ok {
			for _, x := range m {
				n += 1 + count(x)
			}
		}
		return n
	}
	total := count(v)
	refSize := 1
	if total > 255 {
		refSize = 2
	}
	be := binary.BigEndian
	var objs [][]byte
	ref := func(b []byte, i int) []byte {
		if refSize == 1 {
			return append(b, byte(i))
		}
		return be.AppendUint16(b, uint16(i))
	}
	marker := func(kind byte, n int) []byte {
		if n < 15 {
			return []byte{kind<<4 | byte(n)}
		}
		return append([]byte{kind<<4 | 0xF}, bplistInt(int64(n))...)
	}
	var add func(any) (int, error)
	add = func(v any) (int, error) {
		i := len(objs)
		objs = append(objs, nil)
		var b []byte
		switch x := v.(type) {
		case bool:
			b = []byte{0x08}
			if x {
				b[0] = 0x09
			}
		case int:
			b = bplistInt(int64(x))
		case float64:
			b = be.AppendUint64([]byte{0x23}, math.Float64bits(x))
		case []byte:
			b = append(marker(4, len(x)), x...)
		case string:
			if isASCII(x) {
				b = append(marker(5, len(x)), x...)
			} else {
				u := utf16.Encode([]rune(x))
				b = marker(6, len(u))
				for _, c := range u {
					b = be.AppendUint16(b, c)
				}
			}
		case map[string]any:
			keys := sortedKeys(x)
			b = marker(0xD, len(keys))
			var refs []int
			for _, k := range keys {
				r, err := add(k)
				if err != nil {
					return 0, err
				}
				refs = append(refs, r)
			}
			for _, k := range keys {
				r, err := add(x[k])
				if err != nil {
					return 0, err
				}
				refs = append(refs, r)
			}
			for _, r := range refs {
				b = ref(b, r)
			}
		default:
			return 0, fmt.Errorf("bplist: unsupported %T", v)
		}
		objs[i] = b
		return i, nil
	}
	if _, err := add(v); err != nil {
		return nil, err
	}
	out := []byte("bplist00")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(out)
		out = append(out, o...)
	}
	tableOff := len(out)
	offSize := 1
	for tableOff >= 1<<(8*offSize) {
		offSize++
	}
	for _, o := range offsets {
		for k := offSize - 1; k >= 0; k-- {
			out = append(out, byte(o>>(8*k)))
		}
	}
	out = append(out, 0, 0, 0, 0, 0, 0, byte(offSize), byte(refSize))
	out = be.AppendUint64(out, uint64(len(objs)))
	out = be.AppendUint64(out, 0) // top object
	out = be.AppendUint64(out, uint64(tableOff))
	return out, nil
}

func bplistInt(n int64) []byte {
	be := binary.BigEndian
	switch {
	case n >= 0 && n < 1<<8:
		return []byte{0x10, byte(n)}
	case n >= 0 && n < 1<<16:
		return be.AppendUint16([]byte{0x11}, uint16(n))
	case n >= 0 && n < 1<<32:
		return be.AppendUint32([]byte{0x12}, uint32(n))
	}
	return be.AppendUint64([]byte{0x13}, uint64(n))
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// macAliasTarget describes a file on a mounted volume for an alias.
type macAliasTarget struct {
	volumeName string    // e.g. "Hello"
	volumeDate time.Time // the volume's creation date
	mountPoint string    // where users will see it: /Volumes/<name>
	path       string    // the file's path inside the volume: "/.background/background.png"
	parentID   uint32    // catalog node id of the folder holding the file
	fileID     uint32    // catalog node id of the file
	fileDate   time.Time
	ejectable  bool
}

// macTime is a classic Mac OS date: seconds since 1904-01-01 in local
// time, as the Alias Manager's fixed fields hold them (Finder's own
// aliases show the local wall clock there).
func macTime(t time.Time) uint32 {
	if t.IsZero() {
		return 0
	}
	_, offset := t.Zone()
	return uint32(t.Unix() + int64(offset) + 2082844800)
}

// macTimeHighRes is the 8-byte UTC date of alias tags 16 and 17: seconds
// since 1904 as a 48.16 fixed-point number.
func macTimeHighRes(t time.Time) []byte {
	return binary.BigEndian.AppendUint64(nil, uint64(t.Unix()+2082844800)<<16)
}

// pascal is a fixed-size Pascal string field (length byte, bytes, zero
// padding), the text reduced to ASCII.
func pascal(s string, size int) []byte {
	b := make([]byte, size)
	var a []byte
	for _, r := range s {
		if r < 0x80 {
			a = append(a, byte(r))
		} else {
			a = append(a, '?')
		}
	}
	a = a[:min(len(a), size-1)]
	b[0] = byte(len(a))
	copy(b[1:], a)
	return b
}

// encodeMacAlias writes an Alias Manager record, version 2: the 150-byte
// fixed part, then tagged extras (folder name, parent id path, the HFS
// and POSIX paths, the Unicode names, the mount point), ending in tag -1.
func encodeMacAlias(t macAliasTarget) []byte {
	be := binary.BigEndian
	segs := strings.Split(strings.Trim(t.path, "/"), "/")
	file := segs[len(segs)-1]
	b := make([]byte, 4)      // creator code: none
	b = be.AppendUint16(b, 0) // record size, patched below
	b = be.AppendUint16(b, 2) // version
	b = be.AppendUint16(b, 0) // kind: file
	b = append(b, pascal(t.volumeName, 28)...)
	b = be.AppendUint32(b, macTime(t.volumeDate))
	b = append(b, "H+"...)
	drive := uint16(0) // fixed disk
	if t.ejectable {
		drive = 5 // other ejectable media
	}
	b = be.AppendUint16(b, drive)
	b = be.AppendUint32(b, t.parentID)
	b = append(b, pascal(file, 64)...)
	b = be.AppendUint32(b, t.fileID)
	b = be.AppendUint32(b, macTime(t.fileDate))
	b = append(b, 0, 0, 0, 0, 0, 0, 0, 0) // type and creator
	b = be.AppendUint16(b, 0xFFFF)        // nlvlFrom: -1
	b = be.AppendUint16(b, 0xFFFF)        // nlvlTo: -1
	b = be.AppendUint32(b, 0)             // volume attributes
	b = be.AppendUint16(b, 0)             // volume file system id
	b = append(b, make([]byte, 10)...)
	tag := func(id int16, data []byte) {
		b = be.AppendUint16(b, uint16(id))
		b = be.AppendUint16(b, uint16(len(data)))
		b = append(b, data...)
		if len(data)%2 == 1 {
			b = append(b, 0)
		}
	}
	uni := func(s string) []byte {
		u := utf16.Encode([]rune(s))
		d := be.AppendUint16(nil, uint16(len(u)))
		for _, c := range u {
			d = be.AppendUint16(d, c)
		}
		return d
	}
	if len(segs) > 1 {
		tag(0, []byte(segs[len(segs)-2])) // parent folder name
	}
	if !t.volumeDate.IsZero() {
		tag(16, macTimeHighRes(t.volumeDate))
	}
	if !t.fileDate.IsZero() {
		tag(17, macTimeHighRes(t.fileDate))
	}
	tag(1, be.AppendUint32(nil, t.parentID)) // ids of the folders above the file
	// The HFS path: volume and folders joined by ':', then, as in the
	// aliases Finder writes, a NUL before the file name.
	tag(2, []byte(t.volumeName+":"+strings.Join(segs[:len(segs)-1], ":")+":\x00"+file))
	tag(14, uni(file))
	tag(15, uni(t.volumeName))
	tag(18, []byte(t.path))
	tag(19, []byte(t.mountPoint))
	b = be.AppendUint16(b, 0xFFFF)
	b = be.AppendUint16(b, 0)
	be.PutUint16(b[4:], uint16(len(b)))
	return b
}
