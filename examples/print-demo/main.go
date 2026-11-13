// Command print-demo shows package sys/print: print the page through the
// system print dialog, export it as a PDF file, and print a PDF document
// and a generated image.
//
//	go run ./examples/print-demo          # interactive
//	go run ./examples/print-demo -smoke   # export the page as PDF to a temp file, check it, exit
//
// -smoke opens no dialog. TANGO_SMOKE_TIMEOUT_SCALE=3 stretches its
// timeout for slow machines (CI).
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/dialog"
	printing "github.com/tangoui/tango/sys/print"
)

const page = `<!doctype html><meta charset="utf-8"><title>Tango 打印示例</title>
<style>
  body { font: 15px system-ui, sans-serif; margin: 24px; color: #222 }
  table { border-collapse: collapse; margin: 12px 0 }
  td, th { border: 1px solid #bbb; padding: 4px 12px; text-align: left }
  button { margin: 0 8px 8px 0; font: inherit }
  #status { color: #666; min-height: 1.4em }
  @media print { .noprint { display: none } }
</style>
<h2>发票 #42</h2>
<table>
  <tr><th>项目</th><th>数量</th><th>金额</th></tr>
  <tr><td>咖啡豆</td><td>2</td><td>¥ 136.00</td></tr>
  <tr><td>滤纸</td><td>1</td><td>¥ 18.00</td></tr>
  <tr><th colspan="2">合计</th><th>¥ 154.00</th></tr>
</table>
<div class="noprint">
  <button onclick="__tango.post('page')">打印页面…</button>
  <button onclick="__tango.post('save')">导出 PDF…</button>
  <button onclick="__tango.post('pdf')">打印 PDF 文档…</button>
  <button onclick="__tango.post('image')">打印图片…</button>
  <p id="status"></p>
</div>
<script>function setStatus(t) { document.getElementById("status").textContent = t }</script>`

func main() {
	smoke := flag.Bool("smoke", false, "export the page as PDF to a temporary file, check it and exit")
	flag.Parse()

	assets := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	})
	app := tango.New(tango.Options{Name: "Tango Print", ID: "dev.tango.printdemo", Assets: assets})

	app.OnMessage(func(s *tango.Surface, msg string) {
		// The print functions block until the dialog closes; run them off
		// the callback so the page stays responsive meanwhile.
		go func() {
			status := run(app, s, msg)
			s.Eval("setStatus(" + strconv.Quote(status) + ")")
		}()
	})

	app.OnReady(func() {
		s, err := app.NewSurface(tango.SurfaceOptions{Title: "Tango 打印示例", Width: 640, Height: 460, URL: "/", Resizable: true})
		if err != nil {
			log.Fatal(err)
		}
		if *smoke {
			time.AfterFunc(scaled(20*time.Second), func() { log.Print("FAIL timeout"); os.Exit(1) })
			s.OnLoaded(func() { go smokeTest(app, s) })
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func run(app *tango.App, s *tango.Surface, msg string) string {
	switch msg {
	case "page":
		return result("打印", printing.Page(s, printing.Options{Title: "发票 #42"}))
	case "save":
		path, err := dialog.Save(app, dialog.FileOptions{Parent: s, Filename: "发票-42.pdf",
			Filters: []dialog.FileFilter{{Name: "PDF", Patterns: []string{"*.pdf"}}}})
		if err != nil {
			return result("导出", err)
		}
		if err := printing.SavePDF(s, path); err != nil {
			return result("导出", err)
		}
		return "已导出 " + path
	case "pdf":
		// Any PDF works; this one is the page itself.
		doc, err := s.PrintToPDF(tango.PDFOptions{})
		if err != nil {
			return result("生成 PDF", err)
		}
		return result("打印 PDF", printing.PDF(app, doc, printing.Options{Parent: s, Title: "发票 #42.pdf", Copies: 2}))
	case "image":
		return result("打印图片", printing.Image(app, gradient(600, 400), printing.Options{Parent: s, Title: "渐变"}))
	}
	return "未知命令 " + msg
}

func result(what string, err error) string {
	switch {
	case err == nil:
		return what + "：已提交"
	case errors.Is(err, printing.ErrCanceled):
		return what + "：已取消"
	case errors.Is(err, printing.ErrUnsupported):
		return what + "：此平台不支持"
	}
	return what + "失败：" + err.Error()
}

// gradient is a stand-in for any rendered image (a chart, a tango/ui frame).
func gradient(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{uint8(255 * x / w), uint8(255 * y / h), 160, 255})
		}
	}
	return img
}

// smokeTest exports the page and an image to PDF files, checks both and
// quits. It shows no dialog.
func smokeTest(app *tango.App, s *tango.Surface) {
	dir, err := os.MkdirTemp("", "tango-print-demo-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)
	fail := func(f string, a ...any) { log.Printf("FAIL "+f, a...); os.Exit(1) }

	path := filepath.Join(dir, "page.pdf")
	if err := printing.SavePDF(s, path); err != nil {
		fail("SavePDF: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		fail("read: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) || len(data) < 200 {
		fail("not a PDF (%d bytes): %q", len(data), data[:min(len(data), 16)])
	}
	log.Printf("page PDF: %d bytes", len(data))

	doc, err := printing.ImagePDF(gradient(60, 40))
	if err != nil || !printing.IsPDF(doc) {
		fail("ImagePDF: %v", err)
	}
	log.Print("PASS print-demo (page exported as PDF)")
	app.Quit()
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1).
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
