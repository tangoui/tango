package mobile

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
)

// BenchmarkDatePickerOpenFrame measures one frame of an open minute-precision
// date picker popup (the wheels' options must not be rebuilt every frame).
func BenchmarkDatePickerOpenFrame(b *testing.B) {
	open := true
	when := time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC)
	d := phone(func(c *ui.Context) {
		Picker(c, &open, "When").Date(&when, PrecisionMinute).Show()
	}, false)
	d.Frame()
	d.Frame()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Frame()
	}
}
