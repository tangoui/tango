package windows

// Pure logic for the desktop system capabilities (contract sysx_ext.go):
// secure store target names, power and network status mapping, toast XML,
// drag effects and Jump List items. No build tag: tested on every host.
// Identifiers carry an "sx" prefix so they do not collide with other
// files of the package.

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/platform"
)

// ---- Secure store (Credential Manager) ----

// sxCredMaxBlob is CRED_MAX_CREDENTIAL_BLOB_SIZE (5 * 512 bytes).
const sxCredMaxBlob = 5 * 512

var sxErrTooLarge = errors.New("tango: secure store values are limited to 2560 bytes on Windows (Credential Manager)")

// sxAppKey is what identifies the app in system registrations: the app
// ID, else its name, else "tango".
func sxAppKey(id, name string) string {
	switch {
	case id != "":
		return id
	case name != "":
		return name
	}
	return "tango"
}

// sxSecureTarget is the generic credential's TargetName for key.
func sxSecureTarget(id, name, key string) string {
	return sxAppKey(id, name) + ".tango.securestore/" + key
}

// ---- AppUserModelID ----

// sxAUMID derives the process's AppUserModelID: the app ID, else its
// name without spaces; at most 128 characters (SetCurrentProcess-
// ExplicitAppUserModelID's limit). Empty when the app has neither.
func sxAUMID(id, name string) string {
	s := id
	if s == "" {
		s = strings.Join(strings.Fields(name), "")
	}
	for len(s) > 128 {
		_, n := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-n]
	}
	return s
}

// ---- Power ----

// SYSTEM_POWER_STATUS values (winbase.h).
const (
	sxACOffline        = 0
	sxACOnline         = 1
	sxBatteryCharging  = 8
	sxBatteryNoBattery = 128
	sxBatteryUnknown   = 255
	sxPercentUnknown   = 255
	sxSaverOn          = 1
)

// sxPowerStatus maps GetSystemPowerStatus's fields to the contract.
func sxPowerStatus(acLine, batteryFlag, percent, statusFlag byte) platform.PowerStatus {
	st := platform.PowerStatus{Battery: -1, LowPower: statusFlag&sxSaverOn != 0}
	switch acLine {
	case sxACOnline:
		st.Source = platform.PowerSourceAC
	case sxACOffline:
		st.Source = platform.PowerSourceBattery
	}
	if batteryFlag != sxBatteryUnknown && batteryFlag&sxBatteryNoBattery == 0 {
		st.HasBattery = true
		st.Charging = batteryFlag&sxBatteryCharging != 0
		if percent != sxPercentUnknown && percent <= 100 {
			st.Battery = float64(percent) / 100
		}
	}
	if !st.HasBattery && st.Source == platform.PowerSourceBattery {
		st.Source = platform.PowerSourceUnknown // inconsistent report
	}
	return st
}

// SetThreadExecutionState flags (winbase.h).
const (
	sxESSystemRequired  = 0x00000001
	sxESDisplayRequired = 0x00000002
	sxESContinuous      = 0x80000000
)

// sxExecState is the SetThreadExecutionState argument that honours every
// active block (the fallback when power requests are unavailable).
func sxExecState(blocks map[uint64]platform.SleepBlock) uint32 {
	f := uint32(sxESContinuous)
	for _, k := range blocks {
		f |= sxESSystemRequired
		if k == platform.BlockDisplaySleep {
			f |= sxESDisplayRequired
		}
	}
	return f
}

// POWER_REQUEST_TYPE values.
const (
	sxPowerRequestDisplayRequired = 0
	sxPowerRequestSystemRequired  = 1
)

// sxPowerRequestTypes lists the power requests a block sets.
func sxPowerRequestTypes(kind platform.SleepBlock) []uint32 {
	if kind == platform.BlockDisplaySleep {
		return []uint32{sxPowerRequestSystemRequired, sxPowerRequestDisplayRequired}
	}
	return []uint32{sxPowerRequestSystemRequired}
}

// WM_POWERBROADCAST events (winuser.h).
const (
	sxPBTAPMSuspend           = 0x0004
	sxPBTAPMResumeSuspend     = 0x0007
	sxPBTAPMPowerStatusChange = 0x000A
	sxPBTAPMResumeAutomatic   = 0x0012
	sxPBTPowerSettingChange   = 0x8013
)

// sxPowerTracker turns WM_POWERBROADCAST events into contract events:
// Windows sends PBT_APMRESUMEAUTOMATIC and, when a user is present,
// PBT_APMRESUMESUSPEND for one wake; only one PowerResumed is reported.
type sxPowerTracker struct{ asleep, resumed bool }

func (t *sxPowerTracker) event(pbt uint32) (platform.PowerEvent, bool) {
	switch pbt {
	case sxPBTAPMSuspend:
		t.asleep, t.resumed = true, false
		return platform.PowerSuspending, true
	case sxPBTAPMResumeAutomatic, sxPBTAPMResumeSuspend:
		if t.resumed {
			return 0, false
		}
		t.asleep, t.resumed = false, true
		return platform.PowerResumed, true
	case sxPBTAPMPowerStatusChange, sxPBTPowerSettingChange:
		return platform.PowerStatusChanged, true
	}
	return 0, false
}

// ---- Network ----

// NLM_CONNECTIVITY bits (netlistmgr.h).
const (
	sxNLMIPv4Internet = 0x0040
	sxNLMIPv6Internet = 0x0400
	sxNLMIPv4Local    = 0x0020 | 0x0010 // LOCALNETWORK | SUBNET
	sxNLMIPv6Local    = 0x0200 | 0x0100
)

// NLM_CONNECTION_COST bits.
const (
	sxCostUnrestricted     = 0x00000001
	sxCostFixed            = 0x00000002
	sxCostVariable         = 0x00000004
	sxCostOverDataLimit    = 0x00010000
	sxCostCongested        = 0x00020000
	sxCostRoaming          = 0x00040000
	sxCostApproachingLimit = 0x00080000
)

// IANA ifType values used by Windows (ipifcons.h).
const (
	sxIfEthernet    = 6
	sxIfPPP         = 23
	sxIfLoopback    = 24
	sxIfIEEE80211   = 71
	sxIfTunnel      = 131
	sxIfIEEE1394    = 144
	sxIfWWANPP      = 243
	sxIfWWANPP2     = 244
	sxIfPropVirtual = 53
)

// sxIfKind maps an interface type to a network kind.
func sxIfKind(ifType uint32) platform.NetworkKind {
	switch ifType {
	case sxIfEthernet, sxIfIEEE1394:
		return platform.NetworkWired
	case sxIfIEEE80211:
		return platform.NetworkWiFi
	case sxIfWWANPP, sxIfWWANPP2:
		return platform.NetworkCellular
	case 0:
		return platform.NetworkUnknown
	}
	return platform.NetworkOther
}

// sxNetInput is what the Windows APIs reported; the have* flags say which
// sources answered.
type sxNetInput struct {
	haveNLM      bool
	connectivity uint32
	haveCost     bool
	cost         uint32
	haveIf       bool   // a default-route interface was found
	ifType       uint32 // its IfType
	ifUp         bool   // its OperStatus is up
}

// sxNetStatus combines the sources. Without the Network List Manager a
// default route on an operational interface counts as online.
func sxNetStatus(in sxNetInput) platform.NetworkStatus {
	var st platform.NetworkStatus
	switch {
	case in.haveNLM:
		st.Online = in.connectivity&(sxNLMIPv4Internet|sxNLMIPv6Internet) != 0
		local := in.connectivity&(sxNLMIPv4Local|sxNLMIPv6Local) != 0
		if !st.Online && !local {
			st.Kind = platform.NetworkNone
			return st
		}
	case in.haveIf:
		st.Online = in.ifUp
	}
	if !in.haveIf || !in.ifUp {
		if !in.haveNLM {
			st.Kind = platform.NetworkNone
			return st
		}
		st.Kind = platform.NetworkUnknown
	} else {
		st.Kind = sxIfKind(in.ifType)
	}
	if in.haveCost && in.cost != 0 {
		st.Metered = in.cost&(sxCostFixed|sxCostVariable|sxCostRoaming|sxCostOverDataLimit) != 0
		st.Constrained = in.cost&(sxCostApproachingLimit|sxCostOverDataLimit) != 0
	} else if st.Kind == platform.NetworkCellular {
		st.Metered = true
	}
	return st
}

// IP_ADAPTER_ADDRESSES_LH field offsets on 64-bit Windows (iptypes.h):
// Length, IfIndex, Next, AdapterName, FirstUnicast/Anycast/Multicast/
// DnsServerAddress, DnsSuffix, Description, FriendlyName,
// PhysicalAddress[8], PhysicalAddressLength, Flags, Mtu, IfType,
// OperStatus.
const (
	sxAAOffIfIndex    = 4
	sxAAOffNext       = 8
	sxAAOffIfType     = 100
	sxAAOffOperStatus = 104
	sxAAMinSize       = 108
	sxIfOperStatusUp  = 1
)

// ---- Toast notifications ----

// sxXMLEscape escapes text for an XML attribute or element.
func sxXMLEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		case '\'':
			b.WriteString("&apos;")
		default:
			// Control characters other than tab/newline are not allowed in XML 1.0.
			if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sxToastXML is the ToastGeneric payload for n. Empty lines are left out
// (an empty <text> element still takes a line); the title falls back to
// the app name.
func sxToastXML(n platform.Notification, appName string) string {
	title := n.Title
	if title == "" {
		title = appName
	}
	var b strings.Builder
	b.WriteString(`<toast launch="`)
	b.WriteString(sxXMLEscape("tango-notification:" + n.ID))
	b.WriteString(`"><visual><binding template="ToastGeneric">`)
	for _, line := range []string{title, n.Subtitle, n.Body} {
		if line == "" {
			continue
		}
		b.WriteString("<text>")
		b.WriteString(sxXMLEscape(line))
		b.WriteString("</text>")
	}
	b.WriteString(`</binding></visual>`)
	if n.Silent {
		b.WriteString(`<audio silent="true"/>`)
	}
	b.WriteString(`</toast>`)
	return b.String()
}

// ---- Drag out ----

// DROPEFFECT values (oleidl.h).
const (
	sxDropEffectCopy = 1
	sxDropEffectMove = 2
	sxDropEffectLink = 4
)

// sxDragOperation maps DoDragDrop's result and effect.
func sxDragOperation(dropped bool, effect uint32) platform.DragOperation {
	if !dropped {
		return platform.DragNone
	}
	switch {
	case effect&sxDropEffectMove != 0:
		return platform.DragMove
	case effect&sxDropEffectCopy != 0:
		return platform.DragCopy
	case effect&sxDropEffectLink != 0:
		return platform.DragLink
	}
	return platform.DragNone
}

// QueryContinueDrag results (winerror.h) and key state bits (winuser.h).
const (
	sxDragDropSDrop       = 0x00040100
	sxDragDropSCancel     = 0x00040101
	sxDragDropSUseDefault = 0x00040102
	sxMKLButton           = 0x0001
	sxMKRButton           = 0x0002
)

// sxQueryContinue decides IDropSource::QueryContinueDrag: cancel on Esc
// or when the other mouse button joins in, drop when the button that
// started the drag is released, otherwise continue (S_OK).
func sxQueryContinue(escape bool, keys, button uint32) uintptr {
	other := uint32(sxMKRButton)
	if button == sxMKRButton {
		other = sxMKLButton
	}
	switch {
	case escape || keys&other != 0:
		return sxDragDropSCancel
	case keys&button == 0:
		return sxDragDropSDrop
	}
	return 0
}

// ---- WebView2 file drops ----

// sxDropMessage is what the injected drop script posts, with the dropped
// File objects as additional objects.
const sxDropMessage = "__tango_files_dropped__"

// sxDropScript reports drops of files on the page with all their paths:
// chrome.webview.postMessageWithAdditionalObjects passes the File objects,
// which the host reads as ICoreWebView2File. On runtimes without that
// function the script does nothing and the old behaviour (WebView2
// navigates to the first file, the host turns that into a drop) stays.
const sxDropScript = `(function () {
  var wv = window.chrome && window.chrome.webview;
  if (!wv || typeof wv.postMessageWithAdditionalObjects !== "function") return;
  function hasFiles(e) {
    var t = e.dataTransfer && e.dataTransfer.types;
    if (!t) return false;
    for (var i = 0; i < t.length; i++) if (t[i] === "Files") return true;
    return false;
  }
  window.addEventListener("dragover", function (e) {
    if (hasFiles(e)) e.preventDefault();
  }, true);
  window.addEventListener("drop", function (e) {
    if (!hasFiles(e) || !e.dataTransfer.files || !e.dataTransfer.files.length) return;
    e.preventDefault();
    wv.postMessageWithAdditionalObjects("` + sxDropMessage + `", e.dataTransfer.files);
  }, true);
})();`

// ---- Jump List ----

// sxIconLocation splits "path,index" (index may be negative: a resource
// ID). A path without a valid index has index 0.
func sxIconLocation(s string) (string, int) {
	if i := strings.LastIndexByte(s, ','); i > 0 {
		if n, err := strconv.Atoi(strings.TrimSpace(s[i+1:])); err == nil {
			return strings.TrimSpace(s[:i]), n
		}
	}
	return strings.TrimSpace(s), 0
}

// sxTask is one Jump List task as IShellLinkW fields.
type sxTask struct {
	separator   bool
	title       string
	args        string
	description string
	iconPath    string
	iconIndex   int
}

// sxTasks turns shortcut items into tasks. Leading, trailing and doubled
// separators are dropped (the shell would show them as empty lines).
func sxTasks(items []platform.ShortcutItem, exe string) ([]sxTask, error) {
	var out []sxTask
	for _, it := range items {
		if it.Separator {
			if len(out) > 0 && !out[len(out)-1].separator {
				out = append(out, sxTask{separator: true})
			}
			continue
		}
		if it.ID == "" || it.Label == "" {
			return nil, fmt.Errorf("tango: jump list item needs an ID and a label (%q)", it.ID)
		}
		t := sxTask{
			title:       it.Label,
			args:        platform.ShortcutArgPrefix + it.ID,
			description: it.Description,
			iconPath:    exe,
		}
		if it.IconPath != "" {
			t.iconPath, t.iconIndex = sxIconLocation(it.IconPath)
		}
		if strings.ContainsAny(t.args, " \t\"") {
			t.args = `"` + strings.ReplaceAll(t.args, `"`, ``) + `"`
		}
		out = append(out, t)
	}
	for len(out) > 0 && out[len(out)-1].separator {
		out = out[:len(out)-1]
	}
	return out, nil
}
