//go:build darwin && !ios

package darwin

// Native dialogs: NSAlert for messages, NSOpenPanel / NSSavePanel for files.
//
// Every dialog is asynchronous from the caller's point of view: when the
// surface has a visible window it is shown as a sheet of that window
// (-beginSheetModalForWindow:completionHandler:); otherwise panels use
// -beginWithCompletionHandler: and alerts, which have no modeless form, run
// app-modally. Either way the done callback fires exactly once — also when
// the app quits while the dialog is still up (backend.cancelDialogs).

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/rolenames"
)

// NSModalResponse / NSAlertStyle values (NSApplication.h, NSAlert.h).
const (
	nsModalResponseCancel    = 0
	nsModalResponseOK        = 1
	nsAlertFirstButtonReturn = 1000 // the n-th button returns 1000+n

	nsAlertStyleWarning       = 0
	nsAlertStyleInformational = 1
	nsAlertStyleCritical      = 2
)

func alertStyle(k platform.MessageKind) uintptr {
	styles := map[platform.MessageKind]uintptr{
		platform.MessageWarning: nsAlertStyleWarning,
		platform.MessageError:   nsAlertStyleCritical,
	}
	if s, ok := styles[k]; ok {
		return s
	}
	return nsAlertStyleInformational
}

// alertResult turns an alert's modal response into a button index. Any
// response that does not name one of its buttons (endSheet:, abortModal)
// counts as dismissing the alert: an error when there was a real choice to
// make, button 0 for a plain one-button notice.
func alertResult(resp, buttons int) (int, error) {
	index := resp - nsAlertFirstButtonReturn
	if index >= 0 && index < buttons {
		return index, nil
	}
	if buttons > 1 {
		return -1, platform.ErrCanceled
	}
	return 0, nil
}

// filterExtensions collects the distinct lower-case extensions named by the
// filters' glob patterns. A catch-all pattern, or nothing usable at all,
// reports all=true so the panel is left unrestricted.
func filterExtensions(filters []platform.FileFilter) (exts []string, all bool) {
	for _, f := range filters {
		for _, pattern := range f.Patterns {
			ext, anything := patternExtension(pattern)
			if anything {
				return nil, true
			}
			if ext != "" && !slices.Contains(exts, ext) {
				exts = append(exts, ext)
			}
		}
	}
	return exts, len(exts) == 0
}

// patternExtension reads "*.png", ".png" or "png" as "png". Patterns that are
// not a plain extension (wildcards in the middle, paths) give "".
func patternExtension(pattern string) (ext string, anything bool) {
	p := strings.TrimSpace(pattern)
	if p == "*" || p == "*.*" {
		return "", true
	}
	p = strings.TrimPrefix(p, "*")
	p = strings.TrimPrefix(p, ".")
	if strings.ContainsAny(p, "*?/") {
		return "", false
	}
	return strings.ToLower(p), false
}

// parentWindow is the window a dialog for sid should attach to, or 0 when
// the surface is gone, hidden or already showing a sheet.
func (b *backend) parentWindow(sid platform.SurfaceID) id {
	s := b.surfaces[sid]
	if s == nil || s.closed {
		return 0
	}
	if !sendBool(s.win, "isVisible") || send(s.win, "attachedSheet") != 0 {
		return 0
	}
	return s.win
}

// trackDialog remembers how to cancel an open dialog in case the app quits
// first. The returned func forgets it again.
func (b *backend) trackDialog(cancel func()) func() {
	b.dialogSeq++
	key := b.dialogSeq
	b.dialogs[key] = cancel
	return func() { delete(b.dialogs, key) }
}

// dialogSession keeps a dialog object alive from presentation until its
// completion handler runs, and makes sure done sees only the first result
// (a cancel at quit can race with the user's own click).
type dialogSession struct {
	obj     id
	done    func(resp int)
	settled bool
	forget  func()
}

func (b *backend) openSession(obj id, done func(resp int), cancel func()) *dialogSession {
	ds := &dialogSession{obj: retain(obj), done: done}
	ds.forget = b.trackDialog(cancel)
	return ds
}

func (ds *dialogSession) settle(resp int) {
	if ds.settled {
		return
	}
	ds.settled = true
	ds.forget()
	withPool(func() { ds.done(resp) })
	release(ds.obj)
}

// present calls the "…completionHandler:" method sel on target with
// leading args, passing a block that settles the session. The runtime copies
// the block, so our reference is dropped right away.
func (ds *dialogSession) present(target id, selector string, args ...uintptr) {
	handler := objc.NewBlock(func(_ objc.Block, resp int) { ds.settle(resp) })
	send(target, selector, append(args, uintptr(handler))...)
	handler.Release()
}

// beginSheet shows obj (an NSAlert or NSSavePanel) as a sheet on win.
// sheetWin is the sheet's own window, needed to end it on quit.
func (b *backend) beginSheet(obj, win, sheetWin id, done func(resp int)) {
	ds := b.openSession(obj, done, func() {
		send(win, "endSheet:returnCode:", uintptr(sheetWin), nsModalResponseCancel)
	})
	ds.present(obj, "beginSheetModalForWindow:completionHandler:", uintptr(win))
}

func (b *backend) MessageDialog(parent platform.SurfaceID, opts platform.MessageOptions, done func(button int, err error)) {
	withPool(func() {
		alert, buttons := newAlert(opts)
		report := func(resp int) { done(alertResult(resp, buttons)) }
		if win := b.parentWindow(parent); win != 0 {
			b.beginSheet(alert, win, send(alert, "window"), report)
			return
		}
		b.runModalAlert(alert, report)
	})
}

// newAlert builds an autoreleased NSAlert. Without a title the message is
// promoted to the bold headline; with no buttons a localized OK is added.
func newAlert(opts platform.MessageOptions) (alert id, buttons int) {
	alert = autorelease(alloc("NSAlert"))
	send(alert, "setAlertStyle:", alertStyle(opts.Kind))
	headline, detail := opts.Title, opts.Message
	if headline == "" {
		headline, detail = detail, ""
	}
	send(alert, "setMessageText:", uintptr(nsString(headline)))
	if detail != "" {
		send(alert, "setInformativeText:", uintptr(nsString(detail)))
	}
	labels := opts.Buttons
	if len(labels) == 0 {
		labels = []string{rolenames.Text("mac.ok")}
	}
	for _, label := range labels {
		send(alert, "addButtonWithTitle:", uintptr(nsString(label)))
	}
	return alert, len(labels)
}

// runModalAlert runs alert in a nested app-modal loop and reports its
// response. The drain source is in the common modes, so Host.Drain keeps
// running meanwhile. A Quit that arrived during the session aborted it; it is
// carried out after the report, once the outermost session has unwound.
func (b *backend) runModalAlert(alert id, report func(resp int)) {
	send(b.app, "activateIgnoringOtherApps:", boolArg(true))
	b.modalDepth++
	resp := sendInt(alert, "runModal")
	b.modalDepth--
	report(resp)
	if b.modalDepth == 0 && b.quitAfterModal {
		b.quitAfterModal = false
		b.Quit()
	}
}

// setupPanel applies the options shared by open and save panels.
func (b *backend) setupPanel(panel id, opts platform.FileOptions) {
	if t := opts.Title; t != "" {
		// A sheet has no title bar, so the text is repeated as the message
		// shown inside the panel.
		send(panel, "setTitle:", uintptr(nsString(t)))
		send(panel, "setMessage:", uintptr(nsString(t)))
	}
	send(panel, "setCanCreateDirectories:", boolArg(opts.CanCreateDirs))
	send(panel, "setShowsHiddenFiles:", boolArg(opts.ShowHidden))
	if opts.Directory != "" {
		send(panel, "setDirectoryURL:", uintptr(fileURL(opts.Directory)))
	}
	if exts, all := filterExtensions(opts.Filters); !all {
		restrictPanel(panel, exts)
	}
}

// restrictPanel limits the selectable files. macOS 11+ takes UTType objects
// (allowedContentTypes); older systems only know the deprecated extension
// list (allowedFileTypes).
func restrictPanel(panel id, exts []string) {
	if !hasClass("UTType") {
		send(panel, "setAllowedFileTypes:", uintptr(nsArray(mapIDs(exts, nsString)...)))
		return
	}
	utType := class("UTType")
	types := mapIDs(exts, func(e string) id {
		return send(utType, "typeWithFilenameExtension:", uintptr(nsString(e)))
	})
	if len(types) > 0 {
		send(panel, "setAllowedContentTypes:", uintptr(nsArray(types...)))
	}
}

// mapIDs converts each string, dropping nil results.
func mapIDs(in []string, conv func(string) id) []id {
	out := make([]id, 0, len(in))
	for _, s := range in {
		if v := conv(s); v != 0 {
			out = append(out, v)
		}
	}
	return out
}

// runPanel presents an open / save panel without blocking: as a sheet when
// the parent surface has a usable window, else as its own window.
func (b *backend) runPanel(panel id, parent platform.SurfaceID, done func(resp int)) {
	if win := b.parentWindow(parent); win != 0 {
		b.beginSheet(panel, win, panel, done)
		return
	}
	ds := b.openSession(panel, done, func() { send(panel, "cancel:", 0) })
	send(b.app, "activateIgnoringOtherApps:", boolArg(true))
	ds.present(panel, "beginWithCompletionHandler:")
}

func (b *backend) OpenFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(paths []string, err error)) {
	withPool(func() {
		panel := send(class("NSOpenPanel"), "openPanel")
		for selector, on := range map[string]bool{
			"setCanChooseFiles:":          !opts.PickFolders,
			"setCanChooseDirectories:":    opts.PickFolders,
			"setAllowsMultipleSelection:": opts.Multiple,
		} {
			send(panel, selector, boolArg(on))
		}
		b.setupPanel(panel, opts)
		b.runPanel(panel, parent, func(resp int) {
			if resp == nsModalResponseOK {
				done(urlPaths(send(panel, "URLs")), nil)
			} else {
				done(nil, platform.ErrCanceled)
			}
		})
	})
}

// urlPaths returns the file-system paths of an NSArray of file NSURLs.
func urlPaths(urls id) []string {
	var paths []string
	for _, u := range arrayItems(urls) {
		paths = append(paths, goString(send(u, "path")))
	}
	return paths
}

func (b *backend) SaveFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(path string, err error)) {
	withPool(func() {
		panel := send(class("NSSavePanel"), "savePanel")
		if opts.Filename != "" {
			name := splitSaveName(&opts)
			send(panel, "setNameFieldStringValue:", uintptr(nsString(name)))
		}
		b.setupPanel(panel, opts)
		b.runPanel(panel, parent, func(resp int) {
			if resp == nsModalResponseOK {
				done(goString(send(send(panel, "URL"), "path")), nil)
			} else {
				done("", platform.ErrCanceled)
			}
		})
	})
}

// splitSaveName lets Filename carry a full path: when no Directory was given
// and the file's folder exists, that folder becomes the start directory and
// only the base name goes into the name field.
func splitSaveName(opts *platform.FileOptions) string {
	name := opts.Filename
	if opts.Directory != "" || !filepath.IsAbs(name) {
		return name
	}
	dir := filepath.Dir(name)
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		opts.Directory = dir
	}
	return filepath.Base(name)
}
