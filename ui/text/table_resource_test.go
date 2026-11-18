//go:build !darwin || ios || !tango_coretext

package text

import (
	"bytes"
	"io"
	"testing"
	"testing/iotest"

	ot "github.com/go-text/typesetting/font/opentype"
)

func TestShapingTableResourceReadsAcrossEmptyTables(t *testing.T) {
	tables := []ot.Table{
		{Tag: ot.NewTag('t', 'e', 's', '1'), Content: []byte{1, 2, 3}},
		{Tag: ot.NewTag('t', 'e', 's', '2')},
		{Tag: ot.NewTag('t', 'e', 's', '3'), Content: []byte{4, 5}},
	}
	r, err := shapingTableResource(tables)
	if err != nil {
		t.Fatal(err)
	}
	ld, err := ot.NewLoader(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		got, err := ld.RawTable(table.Tag)
		if err != nil || !bytes.Equal(got, table.Content) {
			t.Fatalf("table %s = %v,%v; want %v", table.Tag, got, err, table.Content)
		}
	}
	buf := make([]byte, 4)
	if n, err := r.ReadAt(buf, 12+3*16+2); n != 3 || err != io.EOF || !bytes.Equal(buf[:n], []byte{3, 4, 5}) {
		t.Fatalf("cross-table partial read = %v,%d,%v", buf, n, err)
	}
	if _, err := r.ReadAt(buf, -1); err == nil {
		t.Fatal("negative offset accepted")
	}
	r.Seek(0, io.SeekStart)
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Seek(0, io.SeekStart)
	if err := iotest.TestReader(r, data); err != nil {
		t.Fatal(err)
	}
}
