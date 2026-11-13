package linux

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestPowerStatus(t *testing.T) {
	if st := powerStatus(upowerInfo{}); st.Source != platform.PowerSourceUnknown || st.Battery != -1 {
		t.Errorf("no upower: %+v", st)
	}
	desk := powerStatus(upowerInfo{haveUPower: true, devType: 0, profile: "balanced"})
	if desk.Source != platform.PowerSourceAC || desk.HasBattery || desk.Battery != -1 || desk.LowPower {
		t.Errorf("desktop: %+v", desk)
	}
	lap := powerStatus(upowerInfo{haveUPower: true, onBattery: true, devType: upDeviceBattery, present: true,
		devState: upStateDischarging, percent: 42, profile: "power-saver"})
	if lap.Source != platform.PowerSourceBattery || !lap.HasBattery || lap.Battery != 0.42 || lap.Charging || !lap.LowPower {
		t.Errorf("laptop: %+v", lap)
	}
	ch := powerStatus(upowerInfo{haveUPower: true, devType: upDeviceBattery, present: true, devState: upStatePendingCharge, percent: 130})
	if !ch.Charging || ch.Battery != 1 {
		t.Errorf("charging: %+v", ch)
	}
	if powerStatus(upowerInfo{haveUPower: true, devType: upDeviceBattery, present: true, devState: upStateFullyCharged, percent: 100}).Charging {
		t.Error("full is not charging")
	}
}

func TestInhibitArgs(t *testing.T) {
	if got := logindInhibitArgs("idle", "My 'App'", "why", "block"); got != `('idle', 'My \'App\'', 'why', 'block')` {
		t.Error(got)
	}
	if gsmInhibitFlags(platform.BlockSystemSleep) != 4 || gsmInhibitFlags(platform.BlockDisplaySleep) != 12 {
		t.Error("gsm flags")
	}
}

func TestNMStatus(t *testing.T) {
	cases := []struct {
		state, conn, metered uint32
		typ                  string
		want                 platform.NetworkStatus
	}{
		{20, 1, 0, "", platform.NetworkStatus{Kind: platform.NetworkNone}},
		{40, 0, 0, "802-11-wireless", platform.NetworkStatus{Kind: platform.NetworkNone}},
		{70, 4, 2, "802-3-ethernet", platform.NetworkStatus{Online: true, Kind: platform.NetworkWired}},
		{70, 0, 4, "802-11-wireless", platform.NetworkStatus{Online: true, Kind: platform.NetworkWiFi}},
		{60, 2, 0, "802-11-wireless", platform.NetworkStatus{Kind: platform.NetworkWiFi}},
		{70, 4, 0, "gsm", platform.NetworkStatus{Online: true, Kind: platform.NetworkCellular, Metered: true}},
		{70, 4, 3, "802-11-wireless", platform.NetworkStatus{Online: true, Kind: platform.NetworkWiFi, Metered: true}},
		{70, 4, 1, "vpn", platform.NetworkStatus{Online: true, Kind: platform.NetworkOther, Metered: true}},
		{70, 4, 0, "", platform.NetworkStatus{Online: true, Kind: platform.NetworkUnknown}},
	}
	for _, c := range cases {
		if got := nmStatus(c.state, c.conn, c.metered, c.typ); got != c.want {
			t.Errorf("nmStatus(%d,%d,%d,%q) = %+v, want %+v", c.state, c.conn, c.metered, c.typ, got, c.want)
		}
	}
}

func TestSecretEncoding(t *testing.T) {
	for _, v := range [][]byte{{}, []byte("hello"), {0, 1, 2, 255}} {
		got, err := decodeSecret(encodeSecret(v))
		if err != nil || !bytes.Equal(got, v) || got == nil {
			t.Errorf("round trip %v: %v %v", v, got, err)
		}
	}
	if got, _ := decodeSecret("plain"); string(got) != "plain" {
		t.Error("foreign value", got)
	}
	if _, err := decodeSecret(secretPrefix + "!!"); err == nil {
		t.Error("corrupt accepted")
	}
	if secretApp("", "") != "tango" || secretApp("id", "n") != "id" || secretApp("", "n") != "n" {
		t.Error("secretApp")
	}
	if !secretServiceMissing("GDBus.Error:org.freedesktop.DBus.Error.ServiceUnknown: The name org.freedesktop.secrets was not provided") ||
		secretServiceMissing("Cannot create an item in a locked collection") {
		t.Error("secretServiceMissing")
	}
}

func TestPortalTrigger(t *testing.T) {
	cases := map[string]string{
		"CmdOrCtrl+Shift+K": "CTRL+SHIFT+k",
		"Alt+Space":         "ALT+space",
		"Super+F5":          "LOGO+F5",
		"Shift+Ctrl+Alt+1":  "CTRL+ALT+SHIFT+1",
		"Ctrl++":            "CTRL+plus",
		"PageDown":          "Page_Down",
	}
	for in, want := range cases {
		if got, err := portalTrigger(in); err != nil || got != want {
			t.Errorf("portalTrigger(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "Ctrl+", "Hyper+A", "Ctrl+Nope"} {
		if _, err := portalTrigger(bad); err == nil {
			t.Errorf("portalTrigger(%q) accepted", bad)
		}
	}
}

func TestPortalArgs(t *testing.T) {
	if got := portalRequestPath(":1.42", "tango_3"); got != "/org/freedesktop/portal/desktop/request/1_42/tango_3" {
		t.Error(got)
	}
	if got := createSessionArgs("t1", "s1"); got != "({'handle_token': <'t1'>, 'session_handle_token': <'s1'>},)" {
		t.Error(got)
	}
	got := bindShortcutsArgs("/org/freedesktop/portal/desktop/session/1_42/s1",
		[]portalShortcut{{id: hotkeyShortcutID(7), description: "App: Ctrl+K", trigger: "CTRL+k"}}, "t2")
	want := "(objectpath '/org/freedesktop/portal/desktop/session/1_42/s1', [('tango-hotkey-7', {'description': <'App: Ctrl+K'>, " +
		"'preferred_trigger': <'CTRL+k'>})], '', {'handle_token': <'t2'>})"
	if got != want {
		t.Errorf("bind args\n got %s\nwant %s", got, want)
	}
	if !strings.Contains(bindShortcutsArgs("/s", nil, "t"), "@a(sa{sv}) []") {
		t.Error("empty bind list must be typed")
	}
	if id, ok := hotkeyFromShortcutID(hotkeyShortcutID(42)); !ok || id != 42 {
		t.Error("shortcut id round trip")
	}
	if _, ok := hotkeyFromShortcutID("other-1"); ok {
		t.Error("foreign shortcut id")
	}
}

func TestSNIPixmap(t *testing.T) {
	if sniPixmap(nil) != "@a(iiay) []" || sniPixmap([]byte("not png")) != "@a(iiay) []" {
		t.Error("empty pixmap")
	}
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 0x11, G: 0x22, B: 0x33, A: 0xff})
	img.SetNRGBA(1, 0, color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 0x80})
	var buf bytes.Buffer
	png.Encode(&buf, img)
	want := "[(2, 1, [byte 0xff, 0x11, 0x22, 0x33, 0x80, 0xaa, 0xbb, 0xcc])]"
	if got := sniPixmap(buf.Bytes()); got != want {
		t.Errorf("pixmap = %s", got)
	}
	big := image.NewNRGBA(image.Rect(0, 0, 256, 128))
	px, w, h := argbPixels(big, sniMaxIcon)
	if w != 64 || h != 32 || len(px) != 64*32*4 {
		t.Errorf("scaled %dx%d (%d bytes)", w, h, len(px))
	}
	if sniToolTip("Hi") != "('', @a(iiay) [], 'Hi', '')" {
		t.Error(sniToolTip("Hi"))
	}
	if sniBusName(12, 3) != "org.kde.StatusNotifierItem-12-3" {
		t.Error("bus name")
	}
	if sniItemIsMenu(false, platform.TrayMenuDefault) || !sniItemIsMenu(true, platform.TrayMenuDefault) ||
		!sniItemIsMenu(true, platform.TrayMenuPrimary) || sniItemIsMenu(true, platform.TrayMenuSecondary) {
		t.Error("ItemIsMenu")
	}
}

func TestDBusMenu(t *testing.T) {
	entries := buildMenuEntries([]platform.MenuItem{
		{ID: 10, Label: "&Open", Accelerator: "CmdOrCtrl+O"},
		{Separator: true},
		{Label: "More", Submenu: []platform.MenuItem{
			{ID: 11, Label: "Dark", Checked: true, RadioGroup: "theme"},
			{ID: 12, Label: "Off", Disabled: true},
		}},
		{Role: "quit"},
	}, "App")
	m := buildDBusMenu(entries)
	if !strings.HasPrefix(m.layout, "(0, {'children-display': <'submenu'>}, [<(1, {'label': <'_Open'>, 'shortcut': <[['Control', 'o']]>}, @av [])>, <(2, {'type': <'separator'>}, @av [])>, <(3, ") {
		t.Errorf("layout = %s", m.layout)
	}
	if !strings.Contains(m.layout, "(4, {'label': <'Dark'>, 'toggle-type': <'radio'>, 'toggle-state': <1>}, @av [])") ||
		!strings.Contains(m.layout, "(5, {'label': <'Off'>, 'enabled': <false>}, @av [])") {
		t.Errorf("layout = %s", m.layout)
	}
	if e, ok := m.byID[4]; !ok || e.ID != 11 {
		t.Error("byID[4]", e)
	}
	if e, ok := m.byID[6]; !ok || e.Action != actQuit {
		t.Error("quit item", e)
	}
	if _, ok := m.byID[2]; ok {
		t.Error("separator clickable")
	}
	if _, ok := m.byID[3]; ok {
		t.Error("submenu parent clickable")
	}
	if got := m.subLayout(3, 0); got != "(3, {'label': <'More'>, 'children-display': <'submenu'>}, @av [])" {
		t.Errorf("sub depth 0 = %s", got)
	}
	if got := m.subLayout(3, 1); !strings.Contains(got, "<(4, ") || !strings.Contains(got, "<(5, ") {
		t.Errorf("sub depth 1 = %s", got)
	}
	if got := m.groupProperties([]int32{5, 99}); got != "[(5, {'label': <'Off'>, 'enabled': <false>})]" {
		t.Errorf("group = %s", got)
	}
	if got := m.groupProperties(nil); !strings.HasPrefix(got, "[(0, ") {
		t.Errorf("all group = %s", got)
	}
	if v, ok := m.prop(5, "enabled"); !ok || v != "false" {
		t.Error("prop enabled", v)
	}
	if v, ok := m.prop(1, "visible"); !ok || v != "true" {
		t.Error("prop default", v)
	}
	if _, ok := m.prop(99, "label"); ok {
		t.Error("unknown item")
	}
	empty := buildDBusMenu(nil)
	if empty.layout != "(0, {'children-display': <'submenu'>}, @av [])" || empty.groupProperties([]int32{7}) != "@a(ia{sv}) []" {
		t.Errorf("empty = %s", empty.layout)
	}
	if dbusMenuShortcut("<Control><Shift>n") != "[['Control', 'Shift', 'n']]" || dbusMenuShortcut("") != "" || dbusMenuShortcut("<Control") != "" {
		t.Error("shortcut")
	}
}

func TestStreamHead(t *testing.T) {
	h := http.Header{}
	resp, n := streamHead(h, 0, []byte("<!doctype html><p>hi"), http.MethodGet, true)
	if resp.Status != 200 || !strings.HasPrefix(resp.ContentType, "text/html") || n != 20 {
		t.Errorf("done: %+v %d", resp, n)
	}
	if len(resp.Headers) != 1 || resp.Headers[0] != [2]string{"Content-Length", "20"} {
		t.Errorf("headers %v", resp.Headers)
	}
	sse := http.Header{"Content-Type": {"text/event-stream"}, "Cache-Control": {"no-cache"}}
	resp, n = streamHead(sse, 0, nil, http.MethodGet, false)
	if n != -1 || resp.ContentType != "text/event-stream" || len(resp.Headers) != 1 {
		t.Errorf("sse: %+v %d", resp, n)
	}
	cl := http.Header{"Content-Length": {"1000"}}
	if _, n = streamHead(cl, 206, []byte("x"), http.MethodGet, false); n != 1000 {
		t.Errorf("declared length %d", n)
	}
	bad := http.Header{"Content-Length": {"abc"}}
	resp, n = streamHead(bad, 0, nil, http.MethodGet, false)
	if n != -1 || len(resp.Headers) != 0 {
		t.Errorf("bad length: %+v %d", resp, n)
	}
	if _, n = streamHead(http.Header{}, 0, nil, http.MethodHead, true); n != 0 {
		t.Errorf("HEAD length %d", n)
	}
	if resp, n = streamHead(http.Header{}, 304, nil, http.MethodGet, true); n != 0 || resp.Status != 304 {
		t.Errorf("304: %+v %d", resp, n)
	}
}
