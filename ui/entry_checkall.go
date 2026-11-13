package ui

import (
	"github.com/tangoui/tango/ui/layout"
)

// CheckAll renders a "全选" checkbox (indeterminate when only part of the
// options is selected) above a checkbox group bound to *selected. Disabled
// options keep their state. It reports whether the selection changed.
func CheckAll(c *Context, selected *[]string, opts []Option) bool {
	return checkAll(c, c.Locale().SelectAll, selected, opts)
}

func checkAll(c *Context, label string, selected *[]string, opts []Option) bool {
	changed := false
	Column(c, func() {
		enabled, on := 0, 0
		for _, o := range opts {
			if o.Disabled {
				continue
			}
			enabled++
			if contains(*selected, o.Value) {
				on++
			}
		}
		all := enabled > 0 && on == enabled
		v := all
		if Checkbox(c, label, &v).Indeterminate(on > 0 && !all).Disabled(enabled == 0).Changed() {
			changed = true
			*selected = caSetAll(*selected, opts, !all)
		}
		if CheckboxGroup(c, selected, opts) {
			changed = true
		}
	}, layout.Gap(8))
	return changed
}

// caSetAll checks (on) or unchecks every enabled option, keeping disabled
// options and unknown values untouched and the option order stable.
func caSetAll(sel []string, opts []Option, on bool) []string {
	enabled := map[string]bool{}
	for _, o := range opts {
		if !o.Disabled {
			enabled[o.Value] = true
		}
	}
	var out []string
	for _, v := range sel {
		if !enabled[v] {
			out = append(out, v)
		}
	}
	if on {
		for _, o := range opts {
			if !o.Disabled {
				out = append(out, o.Value)
			}
		}
	}
	return out
}
