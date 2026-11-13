package main

import (
	"bytes"
	"github.com/go-text/typesetting/font"
	"testing"
)

func TestBundledFontContainsChineseGalleryLabels(t *testing.T) {
	f, err := font.ParseTTF(bytes.NewReader(demoChineseFont))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range "按钮输入表格移动图选择文件只读行号撤销重做恢复示例代码高亮" {
		if _, ok := f.NominalGlyph(r); !ok {
			t.Errorf("missing glyph %c", r)
		}
	}
}
