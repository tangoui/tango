//go:build darwin && (ios || tango_gotext)

package text

import (
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"sort"
	"unsafe"
)

// ctTableResource is a virtual sfnt: only its directory is stored in Go memory.
// go-text reads table bytes directly from CoreText into its own parse buffers.
// Checksums are unused by the loader; this resource is never exported as a font.
type ctTableResource struct {
	owner  *ctFontOwner
	header []byte
	tables []ctTableSection
	size   int64
	pos    int64 // Read/Seek, used only while constructing the loader
}

type ctTableSection struct {
	tag          uint32
	offset, size int64
}

func ctTables(f uintptr) *ctTableResource {
	r := &ctTableResource{}
	arr := ctFontCopyAvailableTables(f, 0)
	if arr == 0 {
		return r
	}
	defer cfRelease(arr)
	r.owner = newCTFontOwner(cfRetain(f), cfRelease)
	for i := int64(0); i < cfArrayGetCount(arr); i++ {
		tag := uint32(cfArrayGetValueAtIndex(arr, i))
		if skipTable(tag) {
			continue
		}
		d := ctFontCopyTable(f, tag, 0)
		if d == 0 {
			continue
		}
		n := cfDataGetLength(d)
		cfRelease(d)
		r.tables = append(r.tables, ctTableSection{tag: tag, size: n})
	}
	sort.Slice(r.tables, func(i, j int) bool { return r.tables[i].tag < r.tables[j].tag })
	r.header = make([]byte, 12+16*len(r.tables))
	binary.BigEndian.PutUint32(r.header, 0x00010000)
	binary.BigEndian.PutUint16(r.header[4:], uint16(len(r.tables)))
	r.size = int64(len(r.header))
	for i := range r.tables {
		t := &r.tables[i]
		t.offset = r.size
		dst := r.header[12+16*i:]
		binary.BigEndian.PutUint32(dst, t.tag)
		binary.BigEndian.PutUint32(dst[8:], uint32(t.offset))
		binary.BigEndian.PutUint32(dst[12:], uint32(t.size))
		r.size += (t.size + 3) &^ 3
	}
	return r
}

func (r *ctTableResource) ReadAt(dst []byte, off int64) (int, error) {
	defer runtime.KeepAlive(r.owner)
	if off < 0 {
		return 0, errors.New("negative font offset")
	}
	n := 0
	for len(dst) > 0 {
		if off >= r.size {
			return n, io.EOF
		}
		if off < int64(len(r.header)) {
			count := copy(dst, r.header[off:])
			n, off, dst = n+count, off+int64(count), dst[count:]
			continue
		}
		i := sort.Search(len(r.tables), func(i int) bool { return r.tables[i].offset > off }) - 1
		t := r.tables[i]
		local := off - t.offset
		var count int
		if local >= t.size {
			count = min(len(dst), int(((t.size+3)&^3)-local))
			clear(dst[:count])
		} else {
			d := ctFontCopyTable(r.owner.base, t.tag, 0)
			if d == 0 {
				return n, errors.New("CoreText font table unavailable")
			}
			if cfDataGetLength(d) != t.size {
				cfRelease(d)
				return n, errors.New("CoreText font table length changed")
			}
			data := unsafe.Slice((*byte)(cfDataGetBytePtr(d)), t.size)
			count = copy(dst, data[local:])
			cfRelease(d)
		}
		n, off, dst = n+count, off+int64(count), dst[count:]
	}
	return n, nil
}

func (r *ctTableResource) Read(dst []byte) (int, error) {
	n, err := r.ReadAt(dst, r.pos)
	r.pos += int64(n)
	return n, err
}

func (r *ctTableResource) Seek(offset int64, whence int) (int64, error) {
	base := int64(0)
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = r.pos
	case io.SeekEnd:
		base = r.size
	default:
		return r.pos, errors.New("invalid font seek origin")
	}
	pos := base + offset
	if pos < 0 || offset > 0 && pos < base {
		return r.pos, errors.New("invalid font seek offset")
	}
	r.pos = pos
	return pos, nil
}
