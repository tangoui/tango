package android

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/lane"
)

func TestLaneCapsConditions(t *testing.T) {
	env := func(v string) func(string) string {
		return func(k string) string {
			if k == "TANGO_IPC_LANES_ANDROID" {
				return v
			}
			return ""
		}
	}
	if c := laneCapsFromEnv(env("1"), false); c != nil {
		t.Fatalf("unsupported WebView: %v", c)
	}
	if c := laneCapsFromEnv(env(""), false); c != nil {
		t.Fatalf("unsupported WebView, default: %v", c)
	}
	for _, v := range []string{"0", " OFF ", "false", "False", " off\t"} {
		if c := laneCapsFromEnv(env(v), true); c != nil {
			t.Fatalf("%q: %v", v, c)
		}
	}
	// On by default: unset and empty are on, as is any other value.
	for _, v := range []string{"", "  ", "1", "on", " yes "} {
		if c := laneCapsFromEnv(env(v), true); !slices.Equal(c, []string{lane.DownAndroid, lane.UpAndroid}) {
			t.Fatalf("%q: %v", v, c)
		}
	}
}

// laneTokens returns tokens with a served-and-started current document
// (token 1) at the asset origin, and the raw form of its token.
func laneTokens(t *testing.T) (*docTokens, []byte) {
	t.Helper()
	d := testTokens("https://tango.localhost/")
	tok := d.serve("https://tango.localhost/")
	d.served(tok, "https://tango.localhost/")
	if d.begin("https://tango.localhost/") != "" {
		t.Fatal("served page injected")
	}
	return d, rawTok(t, tok)
}

func rawTok(t *testing.T, tok string) []byte {
	t.Helper()
	b, err := hex.DecodeString(tok)
	if err != nil || len(b) != 16 {
		t.Fatal(tok, err)
	}
	return b
}

// upPkt builds token | gen | packet.
func upPkt(tok []byte, gen uint64, pkt []byte) []byte {
	b := append([]byte{}, tok...)
	b = binary.LittleEndian.AppendUint64(b, gen)
	return append(b, pkt...)
}

func TestLaneUpParse(t *testing.T) {
	d, t1 := laneTokens(t)
	const src = "https://tango.localhost"
	body := []byte{1, 0, 9, 9, 9, 9}
	origin, gen, pkt, probe, ok := parseUp(d, src, upPkt(t1, 7, body))
	if !ok || origin != src || gen != 7 || !bytes.Equal(pkt, body) || probe {
		t.Fatal(origin, gen, pkt, probe, ok)
	}

	// The pending token (served, not started) is accepted too.
	t2s := d.serve("https://tango.localhost/b.html")
	d.served(t2s, "https://tango.localhost/b.html")
	t2 := rawTok(t, t2s)
	if _, _, _, _, ok := parseUp(d, src, upPkt(t2, 1, body)); !ok {
		t.Fatal("pending token refused")
	}

	bad := upPkt(t1, 7, body)
	bad[3] ^= 1
	if _, _, _, _, ok := parseUp(d, src, bad); ok {
		t.Fatal("wrong token accepted")
	}
	if _, _, _, _, ok := parseUp(d, src, upPkt(t1, 7, nil)[:25]); ok {
		t.Fatal("25 bytes accepted")
	}
	if _, _, _, _, ok := parseUp(d, "https://evil.example", upPkt(t1, 7, body)); ok {
		t.Fatal("other source origin accepted")
	}
	if _, _, _, _, ok := parseUp(d, "https://tango.localhost:443", upPkt(t1, 7, body)); !ok {
		t.Fatal("default port refused")
	}

	// The probe: exactly token | gen | 00 00.
	if _, g, p, probe, ok := parseUp(d, src, upPkt(t1, 9, []byte{0, 0})); !ok || !probe || g != 9 || !bytes.Equal(p, []byte{0, 0}) {
		t.Fatal("probe", g, p, probe, ok)
	}
	if _, _, _, probe, ok := parseUp(d, src, upPkt(t1, 9, []byte{0, 0, 5, 5})); !ok || probe {
		t.Fatal("28 bytes starting 00 00 taken for a probe")
	}

	// After the next document starts, the old token is refused.
	d.begin("https://tango.localhost/b.html")
	if _, _, _, _, ok := parseUp(d, src, upPkt(t1, 7, body)); ok {
		t.Fatal("old token accepted after begin")
	}
	if _, _, _, _, ok := parseUp(d, src, upPkt(t2, 7, body)); !ok {
		t.Fatal("promoted token refused")
	}
}

func TestLaneUpOldGenDropped(t *testing.T) {
	d, t1 := laneTokens(t)
	st := newLaneState()
	const src = "https://tango.localhost"
	var calls []uint64
	deliver := func(origin string, gen uint64, p []byte) bool {
		calls = append(calls, gen)
		return gen == 2
	}
	before := LaneStatsSnapshot()
	if c := handleUp(st, d, src, upPkt(t1, 1, []byte{1, 0}), deliver); c != 2 {
		t.Fatal("old generation:", c)
	}
	if c := handleUp(st, d, src, upPkt(t1, 2, []byte{1, 0}), deliver); c != 1 {
		t.Fatal("current generation:", c)
	}
	bad := upPkt(t1, 2, []byte{1, 0})
	bad[0] ^= 0xff
	if c := handleUp(st, d, src, bad, deliver); c != 0 {
		t.Fatal("bad token:", c)
	}
	if c := handleUp(st, d, "https://evil.example", upPkt(t1, 2, []byte{1, 0}), deliver); c != 0 {
		t.Fatal("bad origin:", c)
	}
	if !slices.Equal(calls, []uint64{1, 2}) {
		t.Fatal("deliver calls", calls)
	}
	// A refused probe is answered [1] and does not record a generation.
	if c := handleUp(st, d, src, upPkt(t1, 1, []byte{0, 0}), deliver); c != 2 {
		t.Fatal("refused probe:", c)
	}
	if st.hasProxy {
		t.Fatal("refused probe recorded a generation")
	}
	if c := handleUp(st, d, src, upPkt(t1, 2, []byte{0, 0}), deliver); c != 1 || !st.hasProxy || st.proxyGen != 2 {
		t.Fatal("accepted probe:", c, st.hasProxy, st.proxyGen)
	}
	got := LaneStatsSnapshot()
	if d := got.UpPackets - before.UpPackets; d != 6 {
		t.Fatal("UpPackets", d)
	}
	if d := got.UpBadToken - before.UpBadToken; d != 2 {
		t.Fatal("UpBadToken", d)
	}
	if d := got.UpRefused - before.UpRefused; d != 2 {
		t.Fatal("UpRefused", d)
	}
	if d := got.UpProbes - before.UpProbes; d != 2 {
		t.Fatal("UpProbes", d)
	}
}

func TestLaneQueueCapAndClose(t *testing.T) {
	st := newLaneState()
	chunk := make([]byte, 1<<20)
	for i := 0; i < laneQueueMax/len(chunk); i++ {
		if !st.push(1, chunk) {
			t.Fatal("refused at", i)
		}
	}
	if st.push(1, []byte{1, 2}) {
		t.Fatal("over the cap accepted")
	}
	if q := st.take(); len(q) != laneQueueMax/len(chunk) {
		t.Fatal(len(q))
	}
	if !st.push(1, chunk) {
		t.Fatal("refused after take")
	}
	before := LaneStatsSnapshot().DownQueued
	st.close()
	if st.push(1, []byte{1, 2}) {
		t.Fatal("accepted after close")
	}
	if d := LaneStatsSnapshot().DownQueued - before; d != -1 {
		t.Fatal("DownQueued after close", d)
	}
	if q := st.take(); q != nil {
		t.Fatal("queue kept after close", q)
	}
}

func TestLaneFlushDropsOtherGen(t *testing.T) {
	st := newLaneState()
	p1, p2, p3 := []byte{1, 0, 1}, []byte{1, 0, 2}, []byte{1, 0, 3}
	push := func() {
		st.push(1, p1)
		st.push(2, p2)
		st.push(1, p3)
	}
	var sent [][]byte
	send := func(p []byte) bool { sent = append(sent, p); return true }

	// No accepted probe: nothing goes out.
	push()
	before := LaneStatsSnapshot()
	st.flush(send)
	if len(sent) != 0 {
		t.Fatal("sent without a probe", sent)
	}
	if d := LaneStatsSnapshot().DownDropped - before.DownDropped; d != 3 {
		t.Fatal("DownDropped", d)
	}

	st.probed(2)
	push()
	before = LaneStatsSnapshot()
	st.flush(send)
	if len(sent) != 1 || !bytes.Equal(sent[0], p2) {
		t.Fatal(sent)
	}
	got := LaneStatsSnapshot()
	if d := got.DownDropped - before.DownDropped; d != 2 {
		t.Fatal("DownDropped", d)
	}
	if d := got.DownPackets - before.DownPackets; d != 1 {
		t.Fatal("DownPackets", d)
	}
	if d := got.DownQueued - before.DownQueued; d != -3 {
		t.Fatal("DownQueued", d)
	}

	// A packet the host could not post counts as dropped.
	st.push(2, p2)
	before = LaneStatsSnapshot()
	st.flush(func([]byte) bool { return false })
	if d := LaneStatsSnapshot().DownDropped - before.DownDropped; d != 1 {
		t.Fatal("DownDropped after a failed post", d)
	}
}

func TestLaneBeginKeepsProbedGen(t *testing.T) {
	const src = "https://tango.localhost"
	accept := func(string, uint64, []byte) bool { return true }
	var sent [][]byte
	send := func(p []byte) bool { sent = append(sent, p); return true }

	// Document B (token T2) was served; its probe (gen 2) reaches the main
	// thread before its onPageStarted.
	d, _ := laneTokens(t)
	st := newLaneState()
	t2s := d.serve("https://tango.localhost/b.html")
	d.served(t2s, "https://tango.localhost/b.html")
	if c := handleUp(st, d, src, upPkt(rawTok(t, t2s), 2, []byte{0, 0}), accept); c != 1 {
		t.Fatal(c)
	}
	st.push(2, []byte{1, 0, 7})
	js, keep := beginPage(d, st, "https://tango.localhost/b.html")
	if js != "" || !keep {
		t.Fatal("begin that promoted the probed token:", js, keep)
	}
	st.flush(send)
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{1, 0, 7}) {
		t.Fatal("kept generation not flushed", sent)
	}

	// The next begin (nothing pending) clears the generation.
	if _, keep := beginPage(d, st, "https://tango.localhost/c.html"); keep || st.hasProxy {
		t.Fatal("begin without promotion kept the lane")
	}

	// The probe used the old token T1: begin promoting T2 clears it.
	d, t1 := laneTokens(t)
	st = newLaneState()
	t2s = d.serve("https://tango.localhost/b.html")
	d.served(t2s, "https://tango.localhost/b.html")
	if c := handleUp(st, d, src, upPkt(t1, 1, []byte{0, 0}), accept); c != 1 {
		t.Fatal(c)
	}
	if _, keep := beginPage(d, st, "https://tango.localhost/b.html"); keep || st.hasProxy || st.proxyGen != 0 {
		t.Fatal("begin kept a probe made with the old token", keep, st.hasProxy, st.proxyGen)
	}
	sent = nil
	st.push(1, []byte{1, 0, 8})
	st.flush(send)
	if len(sent) != 0 {
		t.Fatal("flushed after the generation was cleared", sent)
	}
}

func TestPageScriptLaneAPI(t *testing.T) {
	tok := strings.Repeat("0123456789abcdef", 2)
	s := pageScript(tok, "BRIDGE", true)
	for _, want := range []string{"lane", "post: function (u8)", "listen: function (fn)", "fill(0, 0, 16)", "instanceof AB"} {
		if !strings.Contains(s, want) {
			t.Fatalf("lanes=true: no %q in\n%s", want, s)
		}
	}
	// The lane exists only on the postMessage branch.
	jsi := s[strings.Index(s, `typeof ch.post === "function"`):strings.Index(s, `typeof ch.postMessage === "function"`)]
	if strings.Contains(jsi, "lane") {
		t.Fatal("lane on the JavascriptInterface branch")
	}
	if off := pageScript(tok, "BRIDGE", false); strings.Contains(off, "lane") || strings.Contains(off, "listen") {
		t.Fatalf("lanes=false:\n%s", off)
	}
	runLaneShim(t, tok, pageScript(tok, "", true))
}

// runLaneShim runs the shim in node with a fake WebMessageListener channel
// and checks post (token written, buffer cleared afterwards, bad input
// refused) and listen (once; ArrayBuffers only).
func runLaneShim(t *testing.T, tok, script string) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	harness := `
globalThis.window = globalThis;
window.top = window;
globalThis.document = { currentScript: null };
globalThis.location = { href: "https://tango.localhost/" };
globalThis.XMLHttpRequest = class { open() {} setRequestHeader() {} send() {} };
const ls = [], posted = [];
window.__tangoAndroid = {
  addEventListener(k, f) { if (k === "message") ls.push(f); },
  postMessage(d) { posted.push(d instanceof ArrayBuffer ? Array.from(new Uint8Array(d.slice(0))) : d); },
};
` + script + `;
const lane = window.__tangoNative.lane, out = {};
const u8 = new Uint8Array(28); u8[16] = 5; u8[26] = 1;
lane.post(u8);
out.posted = posted;
out.after = Array.from(u8.subarray(0, 16));
out.errs = [];
for (const bad of [new Uint8Array(25), new Uint8Array(new ArrayBuffer(40), 4, 30), new Uint8Array(new ArrayBuffer(40), 0, 30), "x"]) {
  try { lane.post(bad); out.errs.push("none"); } catch (e) { out.errs.push(e.constructor.name); }
}
const got = [];
lane.listen(b => got.push(b.byteLength));
try { lane.listen(() => {}); out.second = "none"; } catch (e) { out.second = "threw"; }
ls.forEach(f => f({ data: new Uint8Array([0]).buffer }));
ls.forEach(f => f({ data: "b1x" }));
out.got = got;
out.frozen = Object.isFrozen(lane);
console.log(JSON.stringify(out));
`
	f := filepath.Join(t.TempDir(), "h.mjs")
	if err := os.WriteFile(f, []byte(harness), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, f).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := `{"posted":[[1,35,69,103,137,171,205,239,1,35,69,103,137,171,205,239,5,0,0,0,0,0,0,0,0,0,1,0]],` +
		`"after":[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],"errs":["TypeError","TypeError","TypeError","TypeError"],` +
		`"second":"threw","got":[1],"frozen":true}`
	if got := strings.TrimSpace(string(out)); got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	_ = tok
}
