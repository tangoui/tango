package linux

// Pure logic of the desktop system capabilities (sysx): power (UPower,
// power-profiles-daemon, logind), network (NetworkManager), the secure
// store (libsecret), the StatusNotifierItem tray with its
// com.canonical.dbusmenu menu, and xdg-desktop-portal GlobalShortcuts.
// No build tag: tested on every host. D-Bus arguments are GVariant text,
// parsed natively with g_variant_parse (no variadic g_variant_new).

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"strconv"
	"strings"

	"github.com/tangoui/tango/internal/platform"
)

// ---------------------------------------------------------------------------
// Power

// UPower device types and states (org.freedesktop.UPower.Device).
const (
	upDeviceBattery = 2

	upStateCharging         = 1
	upStateDischarging      = 2
	upStateEmpty            = 3
	upStateFullyCharged     = 4
	upStatePendingCharge    = 5
	upStatePendingDischarge = 6
)

// upowerInfo is what the backend reads from UPower and the power profile
// daemon.
type upowerInfo struct {
	haveUPower bool
	onBattery  bool    // org.freedesktop.UPower OnBattery
	devType    uint32  // DisplayDevice Type
	devState   uint32  // DisplayDevice State
	percent    float64 // DisplayDevice Percentage, 0..100
	present    bool    // DisplayDevice IsPresent
	profile    string  // ActiveProfile ("power-saver", "balanced", "performance"), "" if none
}

// powerStatus turns UPower's view into the contract's PowerStatus.
func powerStatus(in upowerInfo) platform.PowerStatus {
	st := platform.PowerStatus{Battery: -1, LowPower: in.profile == "power-saver"}
	if !in.haveUPower {
		return st
	}
	st.Source = platform.PowerSourceAC
	if in.onBattery {
		st.Source = platform.PowerSourceBattery
	}
	st.HasBattery = in.present && in.devType == upDeviceBattery
	if st.HasBattery {
		p := in.percent / 100
		st.Battery = min(max(p, 0), 1)
		st.Charging = in.devState == upStateCharging || in.devState == upStatePendingCharge
	}
	return st
}

// logindInhibitArgs are the (ssss) arguments of
// org.freedesktop.login1.Manager.Inhibit.
func logindInhibitArgs(what, who, why, mode string) string {
	return "(" + gvString(what) + ", " + gvString(who) + ", " + gvString(why) + ", " + gvString(mode) + ")"
}

// GNOME session manager inhibit flags (org.gnome.SessionManager.Inhibit).
const (
	gsmInhibitSuspend = 4 // automatic suspend
	gsmInhibitIdle    = 8 // the session becoming idle (screen blank / lock)
)

// gsmInhibitFlags are the GNOME inhibit flags for a sleep block.
func gsmInhibitFlags(kind platform.SleepBlock) uint32 {
	if kind == platform.BlockDisplaySleep {
		return gsmInhibitSuspend | gsmInhibitIdle
	}
	return gsmInhibitSuspend
}

// ---------------------------------------------------------------------------
// Network

// NetworkManager enums (NetworkManager.h).
const (
	nmStateConnectedLocal  = 50
	nmStateConnectedSite   = 60
	nmStateConnectedGlobal = 70

	nmConnectivityFull = 4

	nmMeteredYes      = 1
	nmMeteredGuessYes = 3
)

// nmKind maps an NMActiveConnection Type (a setting name) to a kind.
func nmKind(connType string) platform.NetworkKind {
	switch connType {
	case "802-3-ethernet", "pppoe", "adsl", "infiniband", "bond", "bridge", "vlan", "team", "veth", "macsec":
		return platform.NetworkWired
	case "802-11-wireless", "wifi-p2p", "olpc-mesh", "802-11-olpc-mesh", "wimax":
		return platform.NetworkWiFi
	case "gsm", "cdma":
		return platform.NetworkCellular
	case "":
		return platform.NetworkUnknown
	}
	return platform.NetworkOther // vpn, wireguard, tun, bluetooth, loopback, ...
}

// nmStatus combines NetworkManager's State, Connectivity, Metered and
// PrimaryConnectionType into a NetworkStatus.
func nmStatus(state, connectivity, metered uint32, connType string) platform.NetworkStatus {
	if state < nmStateConnectedLocal {
		return platform.NetworkStatus{Kind: platform.NetworkNone}
	}
	st := platform.NetworkStatus{
		Online:  state == nmStateConnectedGlobal || connectivity == nmConnectivityFull,
		Kind:    nmKind(connType),
		Metered: metered == nmMeteredYes || metered == nmMeteredGuessYes,
	}
	if st.Kind == platform.NetworkCellular {
		st.Metered = true
	}
	return st
}

// ---------------------------------------------------------------------------
// Secure store

// secretSchemaName names the libsecret schema of Tango's items.
const secretSchemaName = "org.tangoui.tango.SecureStore"

// secretApp is the "app" attribute: the app ID, else its name, else "tango".
func secretApp(id, name string) string {
	switch {
	case id != "":
		return id
	case name != "":
		return name
	}
	return "tango"
}

// secretLabel is what keyring managers (Seahorse, KWalletManager) show.
func secretLabel(app, key string) string { return app + " — " + key }

// Secret values are stored as text: libsecret's password API takes a
// NUL-terminated string and the contract stores bytes.
const secretPrefix = "tango-b64:"

func encodeSecret(v []byte) string { return secretPrefix + base64.StdEncoding.EncodeToString(v) }

func decodeSecret(s string) ([]byte, error) {
	enc, ok := strings.CutPrefix(s, secretPrefix)
	if !ok {
		return []byte(s), nil // written by another tool: take it as is
	}
	b, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, fmt.Errorf("tango/linux: corrupt secret: %w", err)
	}
	if b == nil {
		b = []byte{}
	}
	return b, nil
}

// secretServiceMissing reports whether a libsecret error means there is no
// Secret Service on the bus (no gnome-keyring / KWallet / KeePassXC).
func secretServiceMissing(msg string) bool {
	for _, s := range []string{"ServiceUnknown", "was not provided by any .service files", "Cannot autolaunch", "NoReply", "not activatable"} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// GlobalShortcuts portal

// xdgModifiers maps Tango modifier names to the modifiers of the XDG
// shortcuts specification used by preferred_trigger.
var xdgModifiers = map[string]string{
	"cmdorctrl": "CTRL", "commandorcontrol": "CTRL", "ctrl": "CTRL", "control": "CTRL",
	"cmd": "LOGO", "command": "LOGO", "super": "LOGO", "meta": "LOGO",
	"alt": "ALT", "option": "ALT", "shift": "SHIFT",
}

var xdgModifierOrder = []string{"CTRL", "ALT", "SHIFT", "LOGO"}

// portalTrigger converts a Tango accelerator ("CmdOrCtrl+Shift+K") to the
// XDG shortcuts format ("CTRL+SHIFT+k"): modifiers in a fixed order, then
// the xkb keysym name.
func portalTrigger(acc string) (string, error) {
	acc = strings.TrimSpace(acc)
	if acc == "" {
		return "", errors.New("tango/linux: empty accelerator")
	}
	parts := strings.Split(acc, "+")
	if n := len(parts); n >= 2 && parts[n-1] == "" && parts[n-2] == "" {
		parts = append(parts[:n-2], "+")
	}
	key := strings.TrimSpace(parts[len(parts)-1])
	if key == "" {
		return "", fmt.Errorf("tango/linux: accelerator %q has no key", acc)
	}
	mods := map[string]bool{}
	for _, m := range parts[:len(parts)-1] {
		tok, ok := xdgModifiers[strings.ToLower(strings.TrimSpace(m))]
		if !ok {
			return "", fmt.Errorf("tango/linux: accelerator %q: unknown modifier %q", acc, m)
		}
		mods[tok] = true
	}
	name, err := gtkKeyName(key) // GDK keyval names are the xkb keysym names
	if err != nil {
		return "", fmt.Errorf("tango/linux: accelerator %q: %w", acc, err)
	}
	var b strings.Builder
	for _, m := range xdgModifierOrder {
		if mods[m] {
			b.WriteString(m)
			b.WriteByte('+')
		}
	}
	b.WriteString(name)
	return b.String(), nil
}

// portalShortcut is one shortcut handed to BindShortcuts.
type portalShortcut struct {
	id, description, trigger string
}

// bindShortcutsArgs are the (oa(sa{sv})sa{sv}) arguments of
// GlobalShortcuts.BindShortcuts.
func bindShortcutsArgs(session string, list []portalShortcut, token string) string {
	items := make([]string, len(list))
	for i, s := range list {
		items[i] = "(" + gvString(s.id) + ", {'description': <" + gvString(s.description) +
			">, 'preferred_trigger': <" + gvString(s.trigger) + ">})"
	}
	arr := "@a(sa{sv}) []"
	if len(items) > 0 {
		arr = "[" + strings.Join(items, ", ") + "]"
	}
	return "(objectpath " + gvString(session) + ", " + arr + ", '', {'handle_token': <" + gvString(token) + ">})"
}

// createSessionArgs are the (a{sv}) arguments of
// GlobalShortcuts.CreateSession.
func createSessionArgs(token, sessionToken string) string {
	return "({'handle_token': <" + gvString(token) + ">, 'session_handle_token': <" + gvString(sessionToken) + ">},)"
}

// portalRequestPath is the object path of the Request a portal call with
// handle_token token returns, for the caller with unique bus name sender
// (":1.42" -> "1_42").
func portalRequestPath(sender, token string) string {
	s := strings.ReplaceAll(strings.TrimPrefix(sender, ":"), ".", "_")
	return "/org/freedesktop/portal/desktop/request/" + s + "/" + token
}

// hotkeyShortcutID names a hotkey for the portal.
func hotkeyShortcutID(id uint32) string { return "tango-hotkey-" + strconv.FormatUint(uint64(id), 10) }

// hotkeyFromShortcutID is the inverse of hotkeyShortcutID.
func hotkeyFromShortcutID(s string) (uint32, bool) {
	n, ok := strings.CutPrefix(s, "tango-hotkey-")
	if !ok {
		return 0, false
	}
	v, err := strconv.ParseUint(n, 10, 32)
	return uint32(v), err == nil
}

// ---------------------------------------------------------------------------
// StatusNotifierItem

// sniMaxIcon bounds the icon pixmap size sent over D-Bus.
const sniMaxIcon = 64

// sniPixmap converts a PNG to the StatusNotifierItem IconPixmap value
// (a(iiay): width, height, ARGB32 pixels in network byte order), as
// GVariant text. Icons larger than sniMaxIcon are scaled down (nearest
// neighbour; hosts scale again anyway). An empty or broken PNG gives an
// empty array.
func sniPixmap(data []byte) string {
	const empty = "@a(iiay) []"
	if len(data) == 0 {
		return empty
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return empty
	}
	px, w, h := argbPixels(img, sniMaxIcon)
	if w == 0 || h == 0 {
		return empty
	}
	var b strings.Builder
	b.Grow(len(px)*6 + 32)
	fmt.Fprintf(&b, "[(%d, %d, [byte ", w, h)
	for i, v := range px {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "0x%02x", v)
	}
	b.WriteString("])]")
	return b.String()
}

// argbPixels renders img into at most maxSide x maxSide ARGB32 big-endian
// bytes (non-premultiplied, as the specification wants).
func argbPixels(img image.Image, maxSide int) ([]byte, int, int) {
	sb := img.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if w <= 0 || h <= 0 {
		return nil, 0, 0
	}
	nrgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(nrgba, nrgba.Bounds(), img, sb.Min, draw.Src)
	dw, dh := w, h
	if dw > maxSide || dh > maxSide {
		if w >= h {
			dw, dh = maxSide, max(1, h*maxSide/w)
		} else {
			dw, dh = max(1, w*maxSide/h), maxSide
		}
	}
	out := make([]byte, 0, dw*dh*4)
	for y := range dh {
		sy := y * h / dh
		for x := range dw {
			sx := x * w / dw
			o := nrgba.PixOffset(sx, sy)
			r, g, bl, a := nrgba.Pix[o], nrgba.Pix[o+1], nrgba.Pix[o+2], nrgba.Pix[o+3]
			out = append(out, a, r, g, bl)
		}
	}
	return out, dw, dh
}

// sniToolTip is the ToolTip property value (sa(iiay)ss): icon name, icon
// pixmap, title, description.
func sniToolTip(title string) string {
	return "(''" + ", @a(iiay) [], " + gvString(title) + ", '')"
}

// sniItemIsMenu decides ItemIsMenu: true makes the host open the menu on a
// primary click instead of calling Activate.
func sniItemIsMenu(hasMenu bool, on platform.TrayMenuTrigger) bool {
	return hasMenu && on != platform.TrayMenuSecondary
}

// sniBusName is the well-known name an item owns (the specification's
// org.kde.StatusNotifierItem-PID-ID).
func sniBusName(pid, n int) string {
	return "org.kde.StatusNotifierItem-" + strconv.Itoa(pid) + "-" + strconv.Itoa(n)
}

// ---------------------------------------------------------------------------
// com.canonical.dbusmenu

// dbusMenu is a menu laid out for com.canonical.dbusmenu: item ids are
// assigned depth-first from 1 (0 is the root).
type dbusMenu struct {
	layout string                // GetLayout's (ia{sv}av) for the whole tree, GVariant text
	byID   map[int32]menuEntry   // clickable entries by id
	props  map[int32]string      // each item's a{sv}, GVariant text (GetGroupProperties)
	kv     map[int32][][2]string // each item's properties (GetProperty)
	kids   map[int32][]int32     // children ids by parent id
	nodes  map[int32]*dbusMenuNd // for sub-layouts
}

type dbusMenuNd struct {
	id       int32
	props    string
	children []*dbusMenuNd
}

// buildDBusMenu lays entries out for dbusmenu.
func buildDBusMenu(entries []menuEntry) *dbusMenu {
	m := &dbusMenu{byID: map[int32]menuEntry{}, props: map[int32]string{}, kv: map[int32][][2]string{}, kids: map[int32][]int32{}, nodes: map[int32]*dbusMenuNd{}}
	next := int32(0)
	var walk func(es []menuEntry) []*dbusMenuNd
	walk = func(es []menuEntry) []*dbusMenuNd {
		var out []*dbusMenuNd
		for _, e := range es {
			next++
			kv := dbusMenuPropList(e)
			n := &dbusMenuNd{id: next, props: propsText(kv)}
			m.props[n.id] = n.props
			m.kv[n.id] = kv
			m.nodes[n.id] = n
			if len(e.Children) > 0 {
				n.children = walk(e.Children)
			} else if !e.Separator {
				m.byID[n.id] = e
			}
			out = append(out, n)
		}
		return out
	}
	root := &dbusMenuNd{id: 0, props: "{'children-display': <'submenu'>}"}
	root.children = walk(entries)
	m.nodes[0] = root
	m.props[0] = root.props
	m.kv[0] = [][2]string{{"children-display", "'submenu'"}}
	for id, n := range m.nodes {
		for _, c := range n.children {
			m.kids[id] = append(m.kids[id], c.id)
		}
	}
	m.layout = root.text(-1)
	return m
}

// subLayout is the (ia{sv}av) layout of parent to depth levels (-1: all),
// or the root's when parent is unknown.
func (m *dbusMenu) subLayout(parent int32, depth int) string {
	n := m.nodes[parent]
	if n == nil {
		n = m.nodes[0]
	}
	return n.text(depth)
}

func (n *dbusMenuNd) text(depth int) string {
	var kids []string
	if depth != 0 {
		for _, c := range n.children {
			kids = append(kids, "<"+c.text(depth-1)+">")
		}
	}
	arr := "@av []"
	if len(kids) > 0 {
		arr = "[" + strings.Join(kids, ", ") + "]"
	}
	return "(" + strconv.Itoa(int(n.id)) + ", " + n.props + ", " + arr + ")"
}

// dbusMenuPropList is an item's properties as name / GVariant-text
// pairs. Defaults (enabled, visible, type standard) are left out, as the
// spec allows.
func dbusMenuPropList(e menuEntry) [][2]string {
	if e.Separator {
		return [][2]string{{"type", "'separator'"}}
	}
	p := [][2]string{{"label", gvString(e.Label)}}
	if e.Disabled {
		p = append(p, [2]string{"enabled", "false"})
	}
	if len(e.Children) > 0 {
		p = append(p, [2]string{"children-display", "'submenu'"})
	}
	if e.Check {
		tt := "'checkmark'"
		if e.Radio {
			tt = "'radio'"
		}
		state := "0"
		if e.Checked {
			state = "1"
		}
		p = append(p, [2]string{"toggle-type", tt}, [2]string{"toggle-state", state})
	}
	if sc := dbusMenuShortcut(e.Accel); sc != "" {
		p = append(p, [2]string{"shortcut", sc})
	}
	return p
}

func propsText(kv [][2]string) string {
	parts := make([]string, len(kv))
	for i, p := range kv {
		parts[i] = gvString(p[0]) + ": <" + p[1] + ">"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// dbusMenuProps is an item's property dictionary (a{sv}) as text.
func dbusMenuProps(e menuEntry) string { return propsText(dbusMenuPropList(e)) }

// prop returns one property of item id (GetProperty), with the spec's
// defaults for the ones left out.
func (m *dbusMenu) prop(id int32, name string) (string, bool) {
	kv, ok := m.kv[id]
	if !ok {
		return "", false
	}
	for _, p := range kv {
		if p[0] == name {
			return p[1], true
		}
	}
	switch name {
	case "enabled", "visible":
		return "true", true
	case "type":
		return "'standard'", true
	case "label":
		return "''", true
	}
	return "", false
}

// dbusMenuShortcut converts a GTK accelerator ("<Control><Shift>n") to the
// dbusmenu shortcut value (aas): [['Control', 'Shift', 'n']].
func dbusMenuShortcut(accel string) string {
	if accel == "" {
		return ""
	}
	var keys []string
	rest := accel
	for strings.HasPrefix(rest, "<") {
		end := strings.IndexByte(rest, '>')
		if end < 0 {
			return ""
		}
		mod := rest[1:end]
		if mod == "Mod5" {
			mod = "AltGr"
		}
		keys = append(keys, gvString(mod))
		rest = rest[end+1:]
	}
	if rest == "" {
		return ""
	}
	keys = append(keys, gvString(rest))
	return "[[" + strings.Join(keys, ", ") + "]]"
}

// groupProperties is GetGroupProperties' a(ia{sv}) for ids (all items when
// ids is empty), GVariant text.
func (m *dbusMenu) groupProperties(ids []int32) string {
	if len(ids) == 0 {
		for id := range m.props {
			ids = append(ids, id)
		}
		sortInt32(ids)
	}
	var out []string
	for _, id := range ids {
		if p, ok := m.props[id]; ok {
			out = append(out, "("+strconv.Itoa(int(id))+", "+p+")")
		}
	}
	if len(out) == 0 {
		return "@a(ia{sv}) []"
	}
	return "[" + strings.Join(out, ", ") + "]"
}

func sortInt32(s []int32) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
