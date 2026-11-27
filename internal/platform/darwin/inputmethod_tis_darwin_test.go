//go:build darwin && !ios

package darwin

import (
	"slices"
	"testing"
)

func TestIMEObservedInputSource(t *testing.T) {
	const bundle = "com.example.inputmethod.probe"
	source := bundle + ".zh"
	modes := imeModes{bundleID: bundle, ids: []string{"zh", "en"}, current: "zh",
		source: imeInputSource{current: func() string { return source }},
	}
	var changed []string
	for _, selected := range []string{source, bundle + ".en", bundle + ".en", "com.apple.keylayout.ABC", bundle + ".zh"} {
		source = selected
		if mode := modes.observeInputSource(); mode != "" {
			changed = append(changed, mode)
		}
	}
	if modes.current != "zh" || !slices.Equal(changed, []string{"en", "zh"}) {
		t.Fatalf("current mode = %q, changes = %v", modes.current, changed)
	}
}

func TestIMEInputSourceFallback(t *testing.T) {
	const bundle = "com.example.inputmethod.probe"
	zh, en := bundle+".zh", bundle+".en"
	for _, tc := range []struct {
		name, before, after, mode string
		want                      bool
	}{
		{"unchanged owned mode", zh, zh, "en", true},
		{"native request already applied", zh, en, "en", false},
		{"user moved to another input method", zh, "com.apple.keylayout.ABC", "en", false},
		{"foreign mode at request", "com.apple.keylayout.ABC", "com.apple.keylayout.ABC", "en", false},
		{"superseded request", zh, zh, "zh", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls []string
			modes := imeModes{bundleID: bundle, ids: []string{"zh", "en"}, current: tc.mode,
				source: imeInputSource{
					current: func() string { return tc.after },
					selectMode: func(id string) error {
						calls = append(calls, id)
						return nil
					},
				},
			}
			modes.selectInputSourceIfUnchanged("en", tc.before)
			if tc.want && !slices.Equal(calls, []string{en}) || !tc.want && len(calls) != 0 {
				t.Fatalf("TIS selection calls = %v", calls)
			}
		})
	}
}

func TestIMEObservedInputSourceDoesNotUndoPendingChange(t *testing.T) {
	const bundle = "com.example.inputmethod.probe"
	source := bundle + ".zh"
	modes := imeModes{bundleID: bundle, ids: []string{"zh", "en"}, current: "zh",
		source: imeInputSource{current: func() string { return source }},
	}
	modes.observeInputSource()
	modes.current, modes.pending = "en", true
	if changed := modes.observeInputSource(); modes.current != "en" || changed != "" || !modes.pending {
		t.Fatalf("current mode = %q, reported mode = %q, pending = %t", modes.current, changed, modes.pending)
	}
	source = bundle + ".en"
	if changed := modes.observeInputSource(); changed != "" {
		t.Fatalf("local mode was reported as a system change: %q", changed)
	}
}

func TestIMEInputSourceBindings(t *testing.T) {
	load()
	withPool(func() {
		source, err := loadIMEInputSource()
		if err != nil {
			t.Fatal(err)
		}
		if source.notification == 0 {
			t.Error("TIS selection notification not resolved")
		}
		// A test process can be outside a graphical login session.
		_ = source.current()
	})
}
