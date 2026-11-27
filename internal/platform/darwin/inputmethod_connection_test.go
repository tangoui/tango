//go:build darwin && !ios

package darwin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIMEConnectionFromBundle(t *testing.T) {
	load()
	for _, tc := range []struct {
		name, explicit, property, want string
	}{
		{"explicit", "Explicit", "<string>Packaged</string>", "Explicit"},
		{"packaged", "", "<string>Packaged</string>", "Packaged"},
		{"missing", "", "", "com.example.inputmethod.bundle_Connection"},
		{"empty", "", "<string></string>", "com.example.inputmethod.bundle_Connection"},
		{"invalid", "", "<integer>9</integer>", "com.example.inputmethod.bundle_Connection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "IME.app")
			contents := filepath.Join(path, "Contents")
			if err := os.MkdirAll(contents, 0755); err != nil {
				t.Fatal(err)
			}
			plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>com.example.inputmethod.bundle</string>
<key>CFBundlePackageType</key><string>APPL</string>`
			if tc.property != "" {
				plist += "<key>InputMethodConnectionName</key>" + tc.property
			}
			plist += "</dict></plist>"
			if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0644); err != nil {
				t.Fatal(err)
			}
			withPool(func() {
				bundle := send(class("NSBundle"), "bundleWithPath:", uintptr(nsString(path)))
				if bundle == 0 {
					t.Fatal("failed to load test bundle")
				}
				if got := imeConnectionName(tc.explicit, bundle); got != tc.want {
					t.Errorf("connection = %q, want %q", got, tc.want)
				}
			})
		})
	}
}
