// Package updateui is a ready-made update window for apps that use
// service/updater: it checks for updates in the background, shows what a
// new release contains, downloads it with progress, and restarts into it
// when the user agrees.
//
//	u := &updater.Updater{ManifestURL: "...", PublicKey: "...", Version: "1.2.0", AppID: "com.example.notes"}
//	stop, err := updateui.Start(app, u, updateui.Options{}) // after the app is ready
//	...
//	updateui.CheckNow(app, u, updateui.Options{})           // from a "Check for Updates…" menu item
//
// The window is drawn by package ui in a window of its own (uihost.Open),
// so WebView apps and Go-drawn apps use it alike; it follows the app's
// theme, light or dark appearance and language. Start also calls
// Updater.Confirm, so a new version is kept after it starts.
//
// One *updater.Updater should be driven by one of updateui or the jsapi
// updater bridge (jsapi InstallUpdater), not both: the Updater is not
// safe for concurrent use and both install an OnEvent hook.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package updateui
