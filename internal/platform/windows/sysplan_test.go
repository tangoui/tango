package windows

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
	"unicode/utf16"
)

func TestHotkeyPlan(t *testing.T) {
	cases := []struct {
		in   string
		mods uint32
		vk   uint16
	}{
		{"CmdOrCtrl+Shift+K", hkModControl | hkModShift, 'K'},
		{"Super+Space", hkModWin, 0x20},
		{"Win+Alt+F5", hkModWin | hkModAlt, 0x74},
		{"Ctrl++", hkModControl, 0xBB},
		{"Plus", 0, 0xBB},
		{"Meta+1", hkModWin, '1'},
	}
	for _, c := range cases {
		mods, vk, err := hotkeyPlan(c.in)
		if err != nil || mods != c.mods|hkModNoRepeat || vk != c.vk {
			t.Errorf("%q: %#x %#x %v", c.in, mods, vk, err)
		}
	}
	for _, bad := range []string{"", "Hyper+K", "Ctrl+", "Super"} {
		if _, _, err := hotkeyPlan(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func testImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{255, 0, 0, 255})
	img.SetNRGBA(1, 0, color.NRGBA{0, 255, 0, 128})
	img.SetNRGBA(2, 0, color.NRGBA{0, 0, 255, 0})
	img.SetNRGBA(0, 1, color.NRGBA{10, 20, 30, 255})
	img.SetNRGBA(1, 1, color.NRGBA{40, 50, 60, 200})
	img.SetNRGBA(2, 1, color.NRGBA{70, 80, 90, 255})
	return img
}

func decodePNG(t *testing.T, b []byte) *image.NRGBA {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	out := image.NewNRGBA(img.Bounds())
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

func TestDIBRoundTrip(t *testing.T) {
	src := testImage()
	var buf bytes.Buffer
	png.Encode(&buf, src)
	dib, err := pngToDIBV5(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(dib) != 124+3*2*4 || binary.LittleEndian.Uint32(dib) != 124 {
		t.Fatalf("dib len %d", len(dib))
	}
	back, err := dibToPNG(dib)
	if err != nil {
		t.Fatal(err)
	}
	got := decodePNG(t, back)
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if a, b := got.NRGBAAt(x, y), src.NRGBAAt(x, y); a != b && !(a.A == 0 && b.A == 0) {
				t.Errorf("(%d,%d) %v != %v", x, y, a, b)
			}
		}
	}
}

// dib24 builds a top-down 24-bit BITMAPINFOHEADER DIB.
func TestDIB24TopDownAndOpaque32(t *testing.T) {
	le := binary.LittleEndian
	w, h := 2, 2
	stride := 8 // 2*3 rounded to 4
	d := make([]byte, 40+stride*h)
	le.PutUint32(d[0:], 40)
	le.PutUint32(d[4:], uint32(w))
	le.PutUint32(d[8:], uint32(0xFFFFFFFE)) // -2: top-down
	le.PutUint16(d[12:], 1)
	le.PutUint16(d[14:], 24)
	copy(d[40:], []byte{0, 0, 255, 0, 255, 0}) // row 0: red, green (BGR)
	copy(d[48:], []byte{255, 0, 0, 1, 2, 3})   // row 1: blue, (3,2,1)
	img := decodePNG(t, must(dibToPNG(d)))
	if img.NRGBAAt(0, 0) != (color.NRGBA{255, 0, 0, 255}) || img.NRGBAAt(1, 1) != (color.NRGBA{3, 2, 1, 255}) {
		t.Fatalf("%v %v", img.NRGBAAt(0, 0), img.NRGBAAt(1, 1))
	}

	// 32-bit BI_RGB with all-zero alpha reads as opaque; bottom-up.
	d = make([]byte, 40+4)
	le.PutUint32(d[0:], 40)
	le.PutUint32(d[4:], 1)
	le.PutUint32(d[8:], 1)
	le.PutUint16(d[14:], 32)
	copy(d[40:], []byte{30, 20, 10, 0})
	img = decodePNG(t, must(dibToPNG(d)))
	if img.NRGBAAt(0, 0) != (color.NRGBA{10, 20, 30, 255}) {
		t.Fatal(img.NRGBAAt(0, 0))
	}
	if _, err := dibToPNG(d[:30]); err == nil {
		t.Fatal("short DIB accepted")
	}
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}

func TestNotifyIconData(t *testing.T) {
	d := newNotifyIconData(0x1234, 7, nifMessage|nifIcon, 0x8010, 0x99)
	d.setTip("tip")
	d.setBalloon("Title", balloonText("Sub", "Body"), niifInfo|niifNoSound)
	d.setVersion(4)
	le := binary.LittleEndian
	if le.Uint32(d[0:]) != 976 || le.Uint64(d[8:]) != 0x1234 || le.Uint32(d[16:]) != 7 ||
		le.Uint32(d[20:]) != nifMessage|nifIcon|nifInfo || le.Uint32(d[24:]) != 0x8010 || le.Uint64(d[32:]) != 0x99 {
		t.Fatal("header fields")
	}
	read := func(off, n int) string {
		u := make([]uint16, n)
		for i := range u {
			u[i] = le.Uint16(d[off+2*i:])
		}
		return utf16Units(u)
	}
	if read(nidOffTip, 128) != "tip" || read(nidOffInfoTitle, 64) != "Title" || read(nidOffInfo, 256) != "Sub\nBody" {
		t.Fatal("strings")
	}
	if le.Uint32(d[nidOffInfoFlags:]) != niifInfo|niifNoSound || le.Uint32(d[nidOffVersion:]) != 4 {
		t.Fatal("flags")
	}
	long := make([]rune, 300)
	for i := range long {
		long[i] = 'x'
	}
	d.setBalloon(string(long), string(long), 0)
	if len(utf16.Encode([]rune(read(nidOffInfoTitle, 64)))) != 63 || len(read(nidOffInfo, 256)) != 255 {
		t.Fatal("truncation")
	}
	if balloonText("", "") != " " || balloonText("S", "") != "S" {
		t.Fatal("balloonText")
	}
}

func TestDropFiles(t *testing.T) {
	b, err := dropFiles([]string{`C:\a.txt`, `D:\😀`})
	if err != nil {
		t.Fatal(err)
	}
	if binary.LittleEndian.Uint32(b) != dropFilesSize || binary.LittleEndian.Uint32(b[16:]) != 1 {
		t.Fatalf("header: % x", b[:dropFilesSize])
	}
	want := append(utf16.Encode([]rune("C:\\a.txt\x00D:\\😀\x00")), 0)
	got := make([]uint16, (len(b)-dropFilesSize)/2)
	for i := range got {
		got[i] = binary.LittleEndian.Uint16(b[dropFilesSize+2*i:])
	}
	if len(got) != len(want) {
		t.Fatalf("list: %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("list: %v, want %v", got, want)
		}
	}
	if _, err := dropFiles([]string{"a\x00b"}); err == nil {
		t.Error("NUL in path accepted")
	}
}
