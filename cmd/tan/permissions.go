package main

import (
	"fmt"
	"slices"
)

// permission is what one entry of tan.json "permissions" adds to the app.
type permission struct {
	name string
	// iosKeys are the Info.plist usage-description keys. iOS terminates
	// an app that uses the API without them.
	iosKeys []string
	// iosText is the default description, formatted with the app name.
	// Apple rejects vague texts in review; set ios.usageDescriptions.
	iosText string
	android []androidPermission
	// component marks entries that declare an app component rather than a
	// permission (widget): they may have no Android permission.
	component bool
	// androidFeatures are <uses-feature android:required="false"> entries,
	// so that requesting the permission does not hide the app from devices
	// without the hardware on Google Play.
	androidFeatures []string
}

// androidPermission is one <uses-permission>; maxSdk limits it to older
// releases (0: all).
type androidPermission struct {
	name   string
	maxSdk int
}

// permissions is the table behind tan.json "permissions". The names (the
// sys/permission kinds) and entries follow docs/mobile-permissions.md.
var permissions = []permission{
	{
		name:            "camera",
		iosKeys:         []string{"NSCameraUsageDescription"},
		iosText:         "%s uses the camera.",
		android:         []androidPermission{{name: "android.permission.CAMERA"}},
		androidFeatures: []string{"android.hardware.camera"},
	},
	{
		name:    "microphone",
		iosKeys: []string{"NSMicrophoneUsageDescription"},
		iosText: "%s uses the microphone.",
		android: []androidPermission{{name: "android.permission.RECORD_AUDIO"}},
	},
	{
		// Picking photos with the system picker needs no permission; this
		// is for reading the library directly and for saving into it.
		name:    "photos",
		iosKeys: []string{"NSPhotoLibraryUsageDescription", "NSPhotoLibraryAddUsageDescription"},
		iosText: "%s uses your photo library.",
		android: []androidPermission{
			{name: "android.permission.READ_MEDIA_IMAGES"},
			{name: "android.permission.READ_MEDIA_VIDEO"},
			{name: "android.permission.READ_MEDIA_VISUAL_USER_SELECTED"},
			{name: "android.permission.READ_EXTERNAL_STORAGE", maxSdk: 32},
		},
	},
	{
		name:    "location",
		iosKeys: []string{"NSLocationWhenInUseUsageDescription"},
		iosText: "%s uses your location.",
		android: []androidPermission{
			{name: "android.permission.ACCESS_COARSE_LOCATION"},
			{name: "android.permission.ACCESS_FINE_LOCATION"},
		},
		androidFeatures: []string{"android.hardware.location"},
	},
	{
		// Background location: the system only grants it from Settings
		// on Android 11+. Continuous updates in the background also need
		// UIBackgroundModes "location", which tan does not add.
		name:    "location-always",
		iosKeys: []string{"NSLocationWhenInUseUsageDescription", "NSLocationAlwaysAndWhenInUseUsageDescription"},
		iosText: "%s uses your location, also in the background.",
		android: []androidPermission{
			{name: "android.permission.ACCESS_COARSE_LOCATION"},
			{name: "android.permission.ACCESS_FINE_LOCATION"},
			{name: "android.permission.ACCESS_BACKGROUND_LOCATION"},
		},
		androidFeatures: []string{"android.hardware.location"},
	},
	{
		name:    "biometric",
		iosKeys: []string{"NSFaceIDUsageDescription"},
		iosText: "%s uses Face ID to confirm it is you.",
		android: []androidPermission{
			{name: "android.permission.USE_BIOMETRIC"},
			{name: "android.permission.USE_FINGERPRINT", maxSdk: 27},
		},
	},
	{
		// Local and push notifications. iOS asks at run time without an
		// Info.plist key; remote push also needs the aps-environment
		// entitlement from the provisioning profile.
		name:    "notifications",
		android: []androidPermission{{name: "android.permission.POST_NOTIFICATIONS"}},
	},
	{
		// Background tasks (sys/background). iOS: build_ios_app.go adds
		// UIBackgroundModes fetch/processing and the task identifiers
		// <id>.tango.refresh / <id>.tango.processing. Android: the host's
		// JobService needs no permission; persisted jobs (kept across
		// reboots) need RECEIVE_BOOT_COMPLETED.
		name:    "background",
		android: []androidPermission{{name: "android.permission.RECEIVE_BOOT_COMPLETED"}},
	},
	{
		// Home-screen widget (sys/widget): not a permission but a component
		// build_android_app.go declares (org.tango.host.TangoWidget). iOS:
		// no widget yet (docs/specs/mobile-extensions.md).
		name:      "widget",
		component: true,
	},
}

// hasPermission reports whether tan.json "permissions" lists name.
func hasPermission(cfg *Config, name string) bool {
	return slices.Contains(cfg.Permissions, name)
}

func lookupPermission(name string) *permission {
	for i := range permissions {
		if permissions[i].name == name {
			return &permissions[i]
		}
	}
	return nil
}

func permissionNames() []string {
	var names []string
	for _, p := range permissions {
		names = append(names, p.name)
	}
	return names
}

// iosUsageDescriptions maps each Info.plist usage key the app's
// permissions need to its text.
func iosUsageDescriptions(cfg *Config) map[string]string {
	m := map[string]string{}
	for _, name := range cfg.Permissions {
		p := lookupPermission(name)
		if p == nil {
			continue
		}
		text := cfg.IOS.UsageDescriptions[name]
		if text == "" && p.iosText != "" {
			text = fmt.Sprintf(p.iosText, cfg.Name)
		}
		for _, k := range p.iosKeys {
			m[k] = text
		}
	}
	return m
}

// androidPermissions lists the <uses-permission> entries: INTERNET always
// (pages may load remote content), then the app's permissions in order.
func androidPermissions(cfg *Config) ([]androidPermission, []string) {
	perms := []androidPermission{{name: "android.permission.INTERNET"}}
	var features []string
	for _, name := range cfg.Permissions {
		p := lookupPermission(name)
		if p == nil {
			continue
		}
		for _, ap := range p.android {
			if !slices.ContainsFunc(perms, func(q androidPermission) bool { return q.name == ap.name }) {
				perms = append(perms, ap)
			}
		}
		for _, f := range p.androidFeatures {
			if !slices.Contains(features, f) {
				features = append(features, f)
			}
		}
	}
	return perms, features
}
