//go:build darwin && (ios || tango_gotext)

package text

import (
	"bytes"
	"io"
	"runtime"
	"sync"
	"testing"
	"testing/iotest"
	"unsafe"

	ot "github.com/go-text/typesetting/font/opentype"
)

func TestCoreTextTablesAvoidWholeFontCopies(t *testing.T) {
	coreTextOn(t)
	f := uiFontWeighted(Regular, false, 17)
	if f == 0 {
		t.Fatal("system UI font unavailable")
	}
	defer cfRelease(f)
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			tables := ctTables(f)
			runtime.KeepAlive(tables)
		}
	})
	// A table directory needs a few KiB. Copying and checksumming the full
	// system font costs multiple MiB before its shaping tables are parsed.
	if result.AllocedBytesPerOp() > 128<<10 {
		t.Fatalf("font table source allocates %d bytes; want a directory without whole-font copies", result.AllocedBytesPerOp())
	}
}

func TestCoreTextTableResourcePreservesTablesAndOwnsFont(t *testing.T) {
	coreTextOn(t)
	f := uiFontWeighted(Regular, false, 17)
	if f == 0 {
		t.Fatal("system UI font unavailable")
	}
	want := map[ot.Tag][]byte{}
	arr := ctFontCopyAvailableTables(f, 0)
	for i := int64(0); i < cfArrayGetCount(arr); i++ {
		tag := uint32(cfArrayGetValueAtIndex(arr, i))
		if skipTable(tag) {
			continue
		}
		d := ctFontCopyTable(f, tag, 0)
		if d == 0 {
			continue
		}
		want[ot.Tag(tag)] = bytes.Clone(unsafe.Slice((*byte)(cfDataGetBytePtr(d)), cfDataGetLength(d)))
		cfRelease(d)
	}
	cfRelease(arr)
	r := ctTables(f)
	cfRelease(f) // the loader must outlive its caller's CoreText reference
	runtime.GC()
	defer r.owner.release()
	ld, err := ot.NewLoader(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(ld.Tables()) != len(want) {
		t.Fatalf("got %d tables, want %d", len(ld.Tables()), len(want))
	}
	var wg sync.WaitGroup
	for tag, expected := range want {
		wg.Go(func() {
			got, err := ld.RawTable(tag)
			if err != nil || !bytes.Equal(got, expected) {
				t.Errorf("table %s differs from CoreText: %v", tag, err)
			}
		})
	}
	wg.Wait()
	data, err := io.ReadAll(io.NewSectionReader(r, 0, r.size))
	if err != nil {
		t.Fatal(err)
	}
	r.Seek(0, io.SeekStart)
	if err := iotest.TestReader(r, data); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		offset int64
		whence int
	}{{0, io.SeekEnd}, {-7, io.SeekEnd}, {3, io.SeekCurrent}, {r.size + 1, io.SeekStart}, {-1, io.SeekStart}, {0, 42}} {
		reference := bytes.NewReader(data)
		r.Seek(0, io.SeekStart)
		got, err := r.Seek(tc.offset, tc.whence)
		expected, wantErr := reference.Seek(tc.offset, tc.whence)
		if (err != nil) != (wantErr != nil) || err == nil && got != expected {
			t.Fatalf("Seek(%d,%d) = %d,%v; want %d,%v", tc.offset, tc.whence, got, err, expected, wantErr)
		}
	}
	if _, err := r.ReadAt(make([]byte, 1), -1); err == nil {
		t.Fatal("negative ReadAt offset accepted")
	}
}
