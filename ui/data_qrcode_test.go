package ui

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func hexBytes(b []byte) string {
	var s []string
	for _, x := range b {
		s = append(s, fmt.Sprintf("%02X", x))
	}
	return strings.Join(s, " ")
}

// ISO/IEC 18004 Annex I example: "01234567", version 1-M.
func TestQRAnnexVector(t *testing.T) {
	data := qrEncodeData("01234567", qrNumeric, 1, QRLevelM)
	want := []byte{0x10, 0x20, 0x0C, 0x56, 0x61, 0x80, 0xEC, 0x11, 0xEC, 0x11, 0xEC, 0x11, 0xEC, 0x11, 0xEC, 0x11}
	if !bytes.Equal(data, want) {
		t.Fatalf("data codewords\n got %s\nwant %s", hexBytes(data), hexBytes(want))
	}
	ec := qrRemainder(data, qrGenerator(10))
	wantEC := []byte{0xA5, 0x24, 0xD4, 0xC1, 0xED, 0x36, 0xC7, 0x87, 0x2C, 0x55}
	if !bytes.Equal(ec, wantEC) {
		t.Fatalf("ec codewords\n got %s\nwant %s", hexBytes(ec), hexBytes(wantEC))
	}
	m, err := EncodeQR("01234567", QRLevelM)
	if err != nil || m.Version != 1 || m.Size != 21 {
		t.Fatalf("encode: %v %+v", err, m)
	}
}

func TestQRGeneratorPoly(t *testing.T) {
	// Generator of degree 7: α exponents 87 229 146 149 238 102 21 (Annex A).
	g := qrGenerator(7)
	want := []int{87, 229, 146, 149, 238, 102, 21}
	for i, c := range g {
		if int(qrLog[c]) != want[i] {
			t.Fatalf("g7[%d] = α^%d, want α^%d", i, qrLog[c], want[i])
		}
	}
}

func TestQRFormatVersionBits(t *testing.T) {
	if got := QRFormatBits(QRLevelM, 0); got != 0b101010000010010 {
		t.Fatalf("format M/0 = %015b", got)
	}
	if got := QRFormatBits(QRLevelL, 4); got != 0b110011000101111 {
		t.Fatalf("format L/4 = %015b", got)
	}
	if got := QRVersionBits(7); got != 0b000111110010010100 {
		t.Fatalf("version 7 = %018b", got)
	}
	if got := QRVersionBits(40); got != 0b101000110001101001 {
		t.Fatalf("version 40 = %018b", got)
	}
}

func TestQRCapacity(t *testing.T) {
	cases := []struct {
		v    int
		l    QRLevel
		want int
	}{{1, QRLevelL, 19}, {1, QRLevelH, 9}, {7, QRLevelM, 124}, {10, QRLevelQ, 154}, {40, QRLevelL, 2956}, {40, QRLevelH, 1276}, {25, QRLevelM, 1000}}
	for _, c := range cases {
		if got := QRDataCapacity(c.v, c.l); got != c.want {
			t.Errorf("capacity v%d/%d = %d, want %d", c.v, c.l, got, c.want)
		}
	}
	for v := 1; v <= 40; v++ {
		for l := QRLevelL; l <= QRLevelH; l++ {
			if qrRawModules(v)/8 < qrECLen[l][v]*qrBlocks[l][v] {
				t.Fatalf("v%d level %d: negative capacity", v, l)
			}
		}
	}
	if got := qrAlignPositions(7); fmt.Sprint(got) != "[6 22 38]" {
		t.Fatalf("align v7 %v", got)
	}
	if got := qrAlignPositions(32); fmt.Sprint(got) != "[6 34 60 86 112 138]" {
		t.Fatalf("align v32 %v", got)
	}
	if got := qrAlignPositions(36); fmt.Sprint(got) != "[6 24 50 76 102 128 154]" {
		t.Fatalf("align v36 %v", got)
	}
}

func TestQRStructure(t *testing.T) {
	for _, s := range []string{"HELLO WORLD", "https://ant.design", strings.Repeat("中文二维码", 40), strings.Repeat("9", 3000)} {
		for l := QRLevelL; l <= QRLevelH; l++ {
			m, err := EncodeQR(s, l)
			if err != nil {
				t.Fatalf("%q: %v", s[:min(10, len(s))], err)
			}
			if m.Size != 17+4*m.Version {
				t.Fatalf("size %d for v%d", m.Size, m.Version)
			}
			// Finder pattern corners and centers.
			for _, o := range [][2]int{{0, 0}, {m.Size - 7, 0}, {0, m.Size - 7}} {
				if !m.Dark(o[0], o[1]) || !m.Dark(o[0]+3, o[1]+3) || m.Dark(o[0]+1, o[1]+1) {
					t.Fatalf("finder at %v broken", o)
				}
			}
			for i := 8; i < m.Size-8; i++ {
				if m.Dark(i, 6) != (i%2 == 0) || m.Dark(6, i) != (i%2 == 0) {
					t.Fatalf("timing pattern broken at %d", i)
				}
			}
			if !m.Dark(8, m.Size-8) {
				t.Fatal("dark module missing")
			}
			// Format info read back from both copies agrees.
			var a, b uint32
			for i := 0; i < 15; i++ {
				var x1, y1 int
				switch {
				case i <= 5:
					x1, y1 = 8, i
				case i == 6:
					x1, y1 = 8, 7
				case i == 7:
					x1, y1 = 8, 8
				case i == 8:
					x1, y1 = 7, 8
				default:
					x1, y1 = 14-i, 8
				}
				if m.Dark(x1, y1) {
					a |= 1 << uint(i)
				}
				x2, y2 := m.Size-1-i, 8
				if i >= 8 {
					x2, y2 = 8, m.Size-15+i
				}
				if m.Dark(x2, y2) {
					b |= 1 << uint(i)
				}
			}
			if a != b || a != QRFormatBits(l, m.Mask) {
				t.Fatalf("format copies %015b %015b", a, b)
			}
		}
	}
	if _, err := EncodeQR(strings.Repeat("x", 3000), QRLevelH); err != ErrQRTooLong {
		t.Fatalf("want ErrQRTooLong, got %v", err)
	}
}

func TestQRModePick(t *testing.T) {
	if qrPickMode("0123") != qrNumeric || qrPickMode("AB-12") != qrAlphanumeric || qrPickMode("ab") != qrByte {
		t.Fatal("mode pick")
	}
	// 7089 digits is the v40-L numeric maximum.
	m, err := EncodeQR(strings.Repeat("1", 7089), QRLevelL)
	if err != nil || m.Version != 40 {
		t.Fatalf("max numeric: %v", err)
	}
	if _, err := EncodeQR(strings.Repeat("1", 7090), QRLevelL); err == nil {
		t.Fatal("7090 digits should not fit")
	}
}

func TestQRCodeWidget(t *testing.T) {
	refreshed := 0
	d := NewDriver(func(c *Context) {
		if QRCode(c, "https://tango.dev").Status("expired").Refreshed() {
			refreshed++
		}
	}, DriverOptions{Width: 300, Height: 300})
	d.Frame()
	if err := d.ClickLabel("https://tango.dev"); err != nil {
		t.Fatal(err)
	}
	if refreshed != 1 {
		t.Fatalf("refreshed=%d", refreshed)
	}
}
