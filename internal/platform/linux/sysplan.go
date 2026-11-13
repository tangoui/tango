package linux

// Pure logic for the round-2 system integration (no build tag, tested on
// every host): the org.freedesktop.Notifications.Notify arguments as GVariant
// text (parsed with g_variant_parse, which avoids calling the variadic
// g_variant_new through purego), file URIs from the clipboard, and X11
// grab modifiers.

import (
	"errors"
	"fmt"
	"net/url"
	"path" // Linux paths, whatever OS the tests run on
	"strings"
)

// gvString quotes s as a GVariant text-format string literal.
func gvString(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for _, r := range s {
		switch {
		case r == '\'' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7F:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// notifyMarkup escapes text for servers that render body markup (the spec
// lets them; libnotify clients escape the same three characters).
var notifyMarkup = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// notifyArgs builds the (susssasa{sv}i) arguments of Notify. Clicking the
// notification body invokes the "default" action.
func notifyArgs(appName, desktopEntry, title, subtitle, body string, silent bool) string {
	text := body
	if subtitle != "" {
		if text != "" {
			text = subtitle + "\n" + text
		} else {
			text = subtitle
		}
	}
	hints := []string{"'urgency': <byte 1>"}
	if silent {
		hints = append(hints, "'suppress-sound': <true>")
	}
	if desktopEntry != "" {
		hints = append(hints, "'desktop-entry': <"+gvString(desktopEntry)+">")
	}
	return fmt.Sprintf("(%s, uint32 0, '', %s, %s, ['default', 'Open'], {%s}, -1)",
		gvString(appName), gvString(title), gvString(notifyMarkup.Replace(text)), strings.Join(hints, ", "))
}

// uriPaths converts text/uri-list entries to local paths, skipping
// comments and non-file URIs.
func uriPaths(uris []string) []string {
	var out []string
	for _, u := range uris {
		u = strings.TrimSpace(u)
		if u == "" || strings.HasPrefix(u, "#") {
			continue
		}
		p, err := url.Parse(u)
		if err != nil || p.Scheme != "file" || (p.Host != "" && p.Host != "localhost") || p.Path == "" {
			continue
		}
		out = append(out, p.Path)
	}
	return out
}

// fileListData is what ClipboardWriteFiles offers: text/uri-list (one URI
// per CRLF-terminated line, RFC 2483), x-special/gnome-copied-files (the
// action line, then URIs separated by \n; Nautilus, Nemo, Caja, Thunar)
// and plain text (the paths, one per line).
type fileListData struct {
	uriList, gnome, text string
}

func fileListPayloads(paths []string) (fileListData, error) {
	if len(paths) == 0 {
		return fileListData{}, errors.New("tango/linux: no files to copy")
	}
	uris := make([]string, len(paths))
	for i, p := range paths {
		if !path.IsAbs(p) {
			return fileListData{}, fmt.Errorf("tango/linux: clipboard file path is not absolute: %q", p)
		}
		uris[i] = fileURI(path.Clean(p))
	}
	return fileListData{
		uriList: strings.Join(uris, "\r\n") + "\r\n",
		gnome:   "copy\n" + strings.Join(uris, "\n"),
		text:    strings.Join(paths, "\n"),
	}, nil
}

// GDK modifier masks (gdktypes.h) and X11 ones (X.h).
const (
	gdkShiftMask   = 1 << 0
	gdkControlMask = 1 << 2
	gdkMod1Mask    = 1 << 3
	gdkMod4Mask    = 1 << 6
	gdkSuperMask   = 1 << 26
	gdkMetaMask    = 1 << 28

	xShiftMask   = 1 << 0
	xLockMask    = 1 << 1
	xControlMask = 1 << 2
	xMod1Mask    = 1 << 3
	xMod2Mask    = 1 << 4 // NumLock on most layouts
	xMod4Mask    = 1 << 6
)

// x11GrabMods converts the mask gtk_accelerator_parse returns to X11 core
// modifiers (Super and Meta live on Mod4 on common layouts).
func x11GrabMods(gdk uint32) uint32 {
	var m uint32
	if gdk&gdkShiftMask != 0 {
		m |= xShiftMask
	}
	if gdk&gdkControlMask != 0 {
		m |= xControlMask
	}
	if gdk&gdkMod1Mask != 0 {
		m |= xMod1Mask
	}
	if gdk&(gdkMod4Mask|gdkSuperMask|gdkMetaMask) != 0 {
		m |= xMod4Mask
	}
	return m
}

// x11LockVariants are the lock-key states a grab must also cover so the
// hotkey works with CapsLock / NumLock on.
var x11LockVariants = []uint32{0, xLockMask, xMod2Mask, xLockMask | xMod2Mask}

// x11Matches reports whether a key event's state is mods, ignoring locks.
func x11Matches(state, mods uint32) bool {
	const relevant = xShiftMask | xControlMask | xMod1Mask | xMod4Mask
	return state&relevant == mods
}
