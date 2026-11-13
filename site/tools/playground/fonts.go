package main

import (
	_ "embed"

	"github.com/tangoui/tango/ui/text"
)

// The subset includes Chinese glyphs used by the gallery and locale sources.
// See fonts/README.md for upstream provenance, OFL license and reproduction.
//
//go:embed fonts/NotoSansSC-demo.ttf
var demoChineseFont []byte

func init() {
	if err := text.RegisterFont("Noto Sans SC Demo", demoChineseFont); err != nil {
		panic(err)
	}
}
