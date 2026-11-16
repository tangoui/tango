package text

import (
	"encoding/binary"
	"errors"
	"io"
	"sort"

	ot "github.com/go-text/typesetting/font/opentype"
)

// shapingTableResource presents existing table buffers as an sfnt directory.
// The loader ignores checksums, so its private input needs neither checksum
// copies nor a second, concatenated copy of the font. Callers keep table bytes
// immutable for the lifetime of the returned resource.
func shapingTableResource(tables []ot.Table) (ot.Resource, error) {
	if len(tables) > 65535 {
		return nil, errors.New("too many shaping font tables")
	}
	r := &shapingTables{tables: tables, offsets: make([]int64, len(tables))}
	r.header = make([]byte, 12+16*len(tables))
	binary.BigEndian.PutUint32(r.header, uint32(ot.TrueType))
	binary.BigEndian.PutUint16(r.header[4:], uint16(len(tables)))
	size := int64(len(r.header))
	for i, table := range tables {
		if int64(len(table.Content)) > 1<<32-1-size {
			return nil, errors.New("shaping font tables exceed sfnt offset range")
		}
		r.offsets[i] = size
		dst := r.header[12+16*i:]
		binary.BigEndian.PutUint32(dst, uint32(table.Tag))
		binary.BigEndian.PutUint32(dst[8:], uint32(size))
		binary.BigEndian.PutUint32(dst[12:], uint32(len(table.Content)))
		size += int64(len(table.Content))
	}
	return io.NewSectionReader(r, 0, size), nil
}

type shapingTables struct {
	header  []byte
	tables  []ot.Table
	offsets []int64
}

func (r *shapingTables) ReadAt(dst []byte, off int64) (int, error) {
	if off < 0 {
		return 0, errors.New("negative shaping font offset")
	}
	n := 0
	for len(dst) > 0 {
		var data []byte
		if off < int64(len(r.header)) {
			data = r.header[off:]
		} else {
			i := sort.Search(len(r.offsets), func(i int) bool { return r.offsets[i] > off }) - 1
			if i < 0 || off-r.offsets[i] >= int64(len(r.tables[i].Content)) {
				return n, io.EOF
			}
			data = r.tables[i].Content[off-r.offsets[i]:]
		}
		count := copy(dst, data)
		n, off, dst = n+count, off+int64(count), dst[count:]
	}
	return n, nil
}
