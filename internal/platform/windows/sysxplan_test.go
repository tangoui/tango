package windows

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestSxSecureTargetAndAUMID(t *testing.T) {
	if got := sxSecureTarget("com.x.app", "X", "tok"); got != "com.x.app.tango.securestore/tok" {
		t.Fatal(got)
	}
	if got := sxSecureTarget("", "", "k"); got != "tango.tango.securestore/k" {
		t.Fatal(got)
	}
	if got := sxAUMID("", "My  Cool App"); got != "MyCoolApp" {
		t.Fatal(got)
	}
	if got := sxAUMID(strings.Repeat("é", 100), ""); len(got) > 128 || !strings.HasPrefix(got, "é") {
		t.Fatal(len(got))
	}
	if sxAUMID("", "") != "" {
		t.Fatal("expected empty")
	}
}

func TestSxPowerStatus(t *testing.T) {
	cases := []struct {
		ac, flag, pct, sys byte
		want               platform.PowerStatus
	}{
		{1, 128, 255, 0, platform.PowerStatus{Source: platform.PowerSourceAC, Battery: -1}},
		{0, 2, 42, 1, platform.PowerStatus{Source: platform.PowerSourceBattery, HasBattery: true, Battery: 0.42, LowPower: true}},
		{1, 8 | 1, 80, 0, platform.PowerStatus{Source: platform.PowerSourceAC, HasBattery: true, Battery: 0.8, Charging: true}},
		{255, 255, 255, 0, platform.PowerStatus{Battery: -1}},
		{0, 128, 255, 0, platform.PowerStatus{Battery: -1}},
	}
	for i, c := range cases {
		if got := sxPowerStatus(c.ac, c.flag, c.pct, c.sys); got != c.want {
			t.Errorf("%d: %+v, want %+v", i, got, c.want)
		}
	}
}

func TestSxExecStateAndRequests(t *testing.T) {
	if got := sxExecState(nil); got != sxESContinuous {
		t.Fatalf("%#x", got)
	}
	m := map[uint64]platform.SleepBlock{1: platform.BlockSystemSleep}
	if got := sxExecState(m); got != sxESContinuous|sxESSystemRequired {
		t.Fatalf("%#x", got)
	}
	m[2] = platform.BlockDisplaySleep
	if got := sxExecState(m); got != sxESContinuous|sxESSystemRequired|sxESDisplayRequired {
		t.Fatalf("%#x", got)
	}
	if got := sxPowerRequestTypes(platform.BlockDisplaySleep); len(got) != 2 {
		t.Fatal(got)
	}
	if got := sxPowerRequestTypes(platform.BlockSystemSleep); len(got) != 1 || got[0] != sxPowerRequestSystemRequired {
		t.Fatal(got)
	}
}

func TestSxPowerTracker(t *testing.T) {
	var tr sxPowerTracker
	seq := []uint32{sxPBTAPMSuspend, sxPBTAPMResumeAutomatic, sxPBTAPMResumeSuspend, sxPBTAPMPowerStatusChange, 0x9999}
	var got []platform.PowerEvent
	for _, p := range seq {
		if ev, ok := tr.event(p); ok {
			got = append(got, ev)
		}
	}
	want := []platform.PowerEvent{platform.PowerSuspending, platform.PowerResumed, platform.PowerStatusChanged}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatal(got)
		}
	}
	// A second sleep reports a resume again.
	tr.event(sxPBTAPMSuspend)
	if ev, ok := tr.event(sxPBTAPMResumeSuspend); !ok || ev != platform.PowerResumed {
		t.Fatal(ev, ok)
	}
}

func TestSxNetStatus(t *testing.T) {
	cases := []struct {
		name string
		in   sxNetInput
		want platform.NetworkStatus
	}{
		{"offline nlm", sxNetInput{haveNLM: true}, platform.NetworkStatus{Kind: platform.NetworkNone}},
		{"wifi metered", sxNetInput{haveNLM: true, connectivity: sxNLMIPv4Internet, haveCost: true, cost: sxCostVariable, haveIf: true, ifType: sxIfIEEE80211, ifUp: true},
			platform.NetworkStatus{Online: true, Kind: platform.NetworkWiFi, Metered: true}},
		{"wired unrestricted", sxNetInput{haveNLM: true, connectivity: sxNLMIPv6Internet, haveCost: true, cost: sxCostUnrestricted, haveIf: true, ifType: sxIfEthernet, ifUp: true},
			platform.NetworkStatus{Online: true, Kind: platform.NetworkWired}},
		{"local only", sxNetInput{haveNLM: true, connectivity: 0x20, haveIf: true, ifType: sxIfEthernet, ifUp: true},
			platform.NetworkStatus{Kind: platform.NetworkWired}},
		{"cellular no cost", sxNetInput{haveNLM: true, connectivity: sxNLMIPv4Internet, haveIf: true, ifType: sxIfWWANPP, ifUp: true},
			platform.NetworkStatus{Online: true, Kind: platform.NetworkCellular, Metered: true}},
		{"limit", sxNetInput{haveNLM: true, connectivity: sxNLMIPv4Internet, haveCost: true, cost: sxCostFixed | sxCostApproachingLimit, haveIf: true, ifType: sxIfEthernet, ifUp: true},
			platform.NetworkStatus{Online: true, Kind: platform.NetworkWired, Metered: true, Constrained: true}},
		{"no nlm, if up", sxNetInput{haveIf: true, ifType: sxIfTunnel, ifUp: true}, platform.NetworkStatus{Online: true, Kind: platform.NetworkOther}},
		{"no nlm, no route", sxNetInput{}, platform.NetworkStatus{Kind: platform.NetworkNone}},
		{"nlm online, no route info", sxNetInput{haveNLM: true, connectivity: sxNLMIPv4Internet}, platform.NetworkStatus{Online: true, Kind: platform.NetworkUnknown}},
	}
	for _, c := range cases {
		if got := sxNetStatus(c.in); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
	if sxIfKind(0) != platform.NetworkUnknown || sxIfKind(sxIfLoopback) != platform.NetworkOther || sxIfKind(sxIfWWANPP2) != platform.NetworkCellular {
		t.Fatal("ifKind")
	}
}

func TestSxToastXML(t *testing.T) {
	got := sxToastXML(platform.Notification{ID: "n-1", Title: `A & "B"`, Body: "<x>\x01y", Silent: true}, "App")
	want := `<toast launch="tango-notification:n-1"><visual><binding template="ToastGeneric"><text>A &amp; &quot;B&quot;</text><text>&lt;x&gt;y</text></binding></visual><audio silent="true"/></toast>`
	if got != want {
		t.Fatalf("\n%s\n%s", got, want)
	}
	got = sxToastXML(platform.Notification{ID: "2", Subtitle: "s"}, "App")
	if !strings.Contains(got, "<text>App</text><text>s</text>") || strings.Contains(got, "audio") {
		t.Fatal(got)
	}
}

func TestSxDrag(t *testing.T) {
	if sxDragOperation(false, sxDropEffectCopy) != platform.DragNone ||
		sxDragOperation(true, sxDropEffectCopy) != platform.DragCopy ||
		sxDragOperation(true, sxDropEffectMove) != platform.DragMove ||
		sxDragOperation(true, sxDropEffectLink) != platform.DragLink ||
		sxDragOperation(true, 0) != platform.DragNone {
		t.Fatal("dragOperation")
	}
	if sxQueryContinue(true, sxMKLButton, sxMKLButton) != sxDragDropSCancel ||
		sxQueryContinue(false, sxMKLButton, sxMKLButton) != 0 ||
		sxQueryContinue(false, 0, sxMKLButton) != sxDragDropSDrop ||
		sxQueryContinue(false, sxMKLButton|sxMKRButton, sxMKLButton) != sxDragDropSCancel ||
		sxQueryContinue(false, sxMKRButton, sxMKRButton) != 0 {
		t.Fatal("queryContinue")
	}
	b, err := dropFiles([]string{`C:\a.txt`, `C:\b c.txt`})
	if err != nil || len(b) != dropFilesSize+2*(len(`C:\a.txt`)+1+len(`C:\b c.txt`)+1+1) {
		t.Fatal(len(b), err)
	}
	if !strings.Contains(sxDropScript, sxDropMessage) {
		t.Fatal("script")
	}
}

func TestSxTasks(t *testing.T) {
	if p, i := sxIconLocation(`C:\x\app.dll, -101`); p != `C:\x\app.dll` || i != -101 {
		t.Fatal(p, i)
	}
	if p, i := sxIconLocation(`C:\a,b\icon.ico`); p != `C:\a,b\icon.ico` || i != 0 {
		t.Fatal(p, i)
	}
	tasks, err := sxTasks([]platform.ShortcutItem{
		{Separator: true},
		{ID: "new", Label: "New", Description: "d"},
		{Separator: true}, {Separator: true},
		{ID: "x", Label: "X", IconPath: `C:\i.ico,2`},
		{Separator: true},
	}, `C:\app.exe`)
	if err != nil || len(tasks) != 3 || !tasks[1].separator {
		t.Fatal(tasks, err)
	}
	if tasks[0].args != platform.ShortcutArgPrefix+"new" || tasks[0].iconPath != `C:\app.exe` || tasks[2].iconPath != `C:\i.ico` || tasks[2].iconIndex != 2 {
		t.Fatalf("%+v", tasks)
	}
	if _, err := sxTasks([]platform.ShortcutItem{{ID: "", Label: "x"}}, ""); err == nil {
		t.Fatal("empty id accepted")
	}
}
