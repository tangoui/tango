package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// Icons overrides the icon per platform. Every field is optional: what is
// not set is derived from Config.Icon, the master icon.
//
// The master icon is square artwork that fills the whole canvas (no
// transparent margin, no rounded corners), ideally 1024×1024 PNG. That is
// what iOS and Android expect, since they apply their own mask. Desktop
// systems expect the icon to carry its own shape, so tan derives one: a
// rounded square with the platform's margin and a soft shadow (macOS grid,
// also used for Windows and Linux). Give a platform its own file when the
// derived one is not good enough.
//
//	"icons": {
//	  "macos":   "icons/macos.png",
//	  "windows": "icons/app.ico",
//	  "linux":   {"app": "icons/linux.png", "symbolic": "icons/symbolic.svg"},
//	  "ios":     {"dark": "icons/ios-dark.png", "tinted": "icons/ios-tinted.png"},
//	  "android": {"foreground": "icons/fg.png", "background": "#1677FF", "monochrome": "icons/mono.png"},
//	  "notification": "icons/notify.png"
//	}
type Icons struct {
	// MacOS is used as is: 1024×1024 PNG already in the macOS shape (the
	// rounded square on an 824 px grid with transparent margin).
	MacOS string `json:"macos,omitempty"`
	// Windows is a PNG (shaped, transparent background; 256 px or more)
	// or an .ico file used as is.
	Windows string      `json:"windows,omitempty"`
	Linux   LinuxIcons  `json:"linux,omitzero"`
	IOS     IOSIcons    `json:"ios,omitzero"`
	Android AndroidIcon `json:"android,omitzero"`
	// Notification is the Android status-bar icon: white shapes on a
	// transparent background (Android draws it as a mask), ideally 96×96.
	// Without it push notifications use a white silhouette of the app icon.
	Notification string `json:"notification,omitempty"`
}

// LinuxIcons are the freedesktop icons.
type LinuxIcons struct {
	App      string `json:"app,omitempty"`      // PNG used instead of the derived shape
	Symbolic string `json:"symbolic,omitempty"` // monochrome SVG for panels (<exe>-symbolic.svg)
}

// IOSIcons are the iOS 18 appearance variants of the app icon (1024 PNG).
type IOSIcons struct {
	Dark   string `json:"dark,omitempty"`   // dark appearance: artwork on a transparent or dark background
	Tinted string `json:"tinted,omitempty"` // tinted appearance: grayscale artwork the system colours
}

// AndroidIcon is the adaptive launcher icon (API 26+). Layers are 108 dp
// squares of which the launcher shows the centre 72 dp (the inner 66 dp is
// always visible, whatever the mask).
type AndroidIcon struct {
	// Foreground PNG with transparency, ideally 432×432; keep the artwork
	// inside the centre 66/108. Default: the master icon fitted to the
	// visible 72 dp.
	Foreground string `json:"foreground,omitempty"`
	// Background is a PNG or a colour "#RRGGBB". Default: the colour of the
	// master icon's border.
	Background string `json:"background,omitempty"`
	// Monochrome PNG for Android 13 themed icons (alpha only, same layout
	// as Foreground). Default: none (themed icons are skipped).
	Monochrome string `json:"monochrome,omitempty"`
}

var hexColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// IsColor reports whether Background is a colour rather than a file.
func (a AndroidIcon) IsColor() bool { return strings.HasPrefix(a.Background, "#") }

func (ic Icons) validate() []error {
	var errs []error
	file := func(key, p string, exts ...string) {
		if p == "" {
			return
		}
		if configPathAbs(p) {
			errs = append(errs, fmt.Errorf(`"icons.%s": path %q must be relative to the project`, key, p))
		}
		ext := strings.ToLower(filepath.Ext(p))
		for _, e := range exts {
			if ext == e {
				return
			}
		}
		errs = append(errs, fmt.Errorf(`"icons.%s": %q must be a %s file`, key, p, strings.Join(exts, " or ")))
	}
	file("macos", ic.MacOS, ".png")
	file("windows", ic.Windows, ".png", ".ico")
	file("linux.app", ic.Linux.App, ".png")
	file("linux.symbolic", ic.Linux.Symbolic, ".svg")
	file("ios.dark", ic.IOS.Dark, ".png")
	file("ios.tinted", ic.IOS.Tinted, ".png")
	file("android.foreground", ic.Android.Foreground, ".png")
	file("android.monochrome", ic.Android.Monochrome, ".png")
	file("notification", ic.Notification, ".png")
	if bg := ic.Android.Background; ic.Android.IsColor() {
		if !hexColorPattern.MatchString(bg) {
			errs = append(errs, fmt.Errorf(`"icons.android.background": %q must be a PNG path or a colour like #1677FF`, bg))
		}
	} else {
		file("android.background", bg, ".png")
	}
	return errs
}
