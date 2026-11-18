package text

import "testing"

// TestNumericWidthIndependentOfDirection: a digits-only string has no strong
// characters, so its advance width must not depend on the base direction it
// is laid out with. The numeric fast path (LTR only) sums unrounded
// font-unit advances, while the RTL path shapes (HarfBuzz, 26.6 advances),
// so the same label measures differently by ~0.01px. Callers that measure
// with Width (LTR) and paint with DirRTL (RTL InputNumber) misplace it.
func TestNumericWidthIndependentOfDirection(t *testing.T) {
	for _, s := range []string{"42", "421", "1", "12.34"} {
		st := Style{Size: 14, LineHeight: 22}
		ltr := Width(s, st)
		rtl := Shape(s, st, Options{NoWrap: true, Direction: DirRTL}).Width
		if ltr != rtl {
			t.Errorf("%q: Width (LTR) = %v, Shape(DirRTL).Width = %v (diff %v)", s, ltr, rtl, rtl-ltr)
		}
	}
}
