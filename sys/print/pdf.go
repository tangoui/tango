package print

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image"
	"strconv"
)

// pointsPerPixel maps image pixels to PDF points as a 96-dpi screen would
// (72 points per inch). The print dialog scales the page to the paper.
const pointsPerPixel = 72.0 / 96.0

// ImagePDF returns a PDF document with one page per image, each page the
// size of its image at 96 dpi. Transparent pixels are composited on
// white. It is what File and Image print; it also turns a rendered
// tango/ui frame (or any image.Image) into a PDF to print or save.
func ImagePDF(imgs ...image.Image) ([]byte, error) {
	if len(imgs) == 0 {
		return nil, errors.New("tango/print: no image")
	}
	var w pdfWriter
	w.buf.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n") // binary marker comment (PDF 1.7 §7.5.2)

	// Object numbers: 1 catalog, 2 page tree, then three per page
	// (page, content stream, image).
	n := len(imgs)
	kids := make([]byte, 0, n*8)
	for i := range n {
		kids = strconv.AppendInt(kids, int64(3+3*i), 10)
		kids = append(kids, " 0 R "...)
	}
	w.object(1, "<< /Type /Catalog /Pages 2 0 R >>")
	w.object(2, fmt.Sprintf("<< /Type /Pages /Count %d /Kids [ %s] >>", n, kids))
	for i, img := range imgs {
		b := img.Bounds()
		if b.Empty() {
			return nil, fmt.Errorf("tango/print: image %d is empty", i)
		}
		pw, ph := float64(b.Dx())*pointsPerPixel, float64(b.Dy())*pointsPerPixel
		page, content, xobj := 3+3*i, 4+3*i, 5+3*i
		w.object(page, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s %s] "+
			"/Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>", num(pw), num(ph), xobj, content))
		w.stream(content, "", []byte(fmt.Sprintf("q %s 0 0 %s 0 0 cm /Im0 Do Q", num(pw), num(ph))))
		pix, err := deflateRGB(img)
		if err != nil {
			return nil, err
		}
		w.stream(xobj, fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d "+
			"/ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode", b.Dx(), b.Dy()), pix)
	}
	w.finish(1)
	return w.buf.Bytes(), nil
}

// num formats a dimension with at most two decimals, as PDF real numbers
// have no exponent form.
func num(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

// deflateRGB returns img's pixels as zlib-compressed 8-bit RGB rows, top
// row first (the PDF image space has its origin at the top-left).
func deflateRGB(img image.Image) ([]byte, error) {
	b := img.Bounds()
	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	row := make([]byte, 3*b.Dx())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for i, x := 0, b.Min.X; x < b.Max.X; i, x = i+3, x+1 {
			r, g, bl, a := img.At(x, y).RGBA() // alpha-premultiplied, 16 bit
			white := 0xffff - a
			row[i] = byte((r + white) >> 8)
			row[i+1] = byte((g + white) >> 8)
			row[i+2] = byte((bl + white) >> 8)
		}
		if _, err := zw.Write(row); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// pdfWriter writes numbered indirect objects and the cross-reference
// table (PDF 1.7, ISO 32000-1 §7.5).
type pdfWriter struct {
	buf     bytes.Buffer
	offsets []int // offsets[i] is object i+1's byte offset
}

func (w *pdfWriter) begin(num int) {
	for len(w.offsets) < num {
		w.offsets = append(w.offsets, 0)
	}
	w.offsets[num-1] = w.buf.Len()
	fmt.Fprintf(&w.buf, "%d 0 obj\n", num)
}

func (w *pdfWriter) object(num int, body string) {
	w.begin(num)
	w.buf.WriteString(body)
	w.buf.WriteString("\nendobj\n")
}

func (w *pdfWriter) stream(num int, dict string, data []byte) {
	w.begin(num)
	fmt.Fprintf(&w.buf, "<< %s /Length %d >>\nstream\n", dict, len(data))
	w.buf.Write(data)
	w.buf.WriteString("\nendstream\nendobj\n")
}

func (w *pdfWriter) finish(root int) {
	xref := w.buf.Len()
	fmt.Fprintf(&w.buf, "xref\n0 %d\n0000000000 65535 f \n", len(w.offsets)+1)
	for _, off := range w.offsets {
		fmt.Fprintf(&w.buf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&w.buf, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(w.offsets)+1, root, xref)
}
