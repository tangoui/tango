package tango

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/internal/downq"
)

// downHarness exercises the downShim part of coreShim in node. argv[2] is
// the shim file, argv[3] a JSON file {"batches": {name: base64}} of poll
// response bodies made by downq.EncodeBatch, argv[4] the scenario to run.
// fetch is a fake whose calls stay pending until the scenario answers them.
const downHarness = `
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const shim = fs.readFileSync(process.argv[2], "utf8");
const data = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
const PREFIX = "\u0000tango-int:down:";

const thrown = [];
process.on("uncaughtException", (e) => thrown.push(e.message));

function setup(trace = false) {
  const posted = [];
  const fetches = [];
  const traceEvents = [];
  const ctx = { setTimeout, Promise, console, TextDecoder, atob };
  if (trace) {
    ctx.__tangoIpcTrace = {
      enabled: true,
      event(name, fields) { traceEvents.push({ name, ...(fields || {}) }); },
    };
  }
  ctx.window = ctx;
  ctx.__tangoNative = { post(s) { posted.push(s); } };
  ctx.fetch = (url, opts) => new Promise((resolve, reject) => fetches.push({ url, opts, resolve, reject }));
  vm.createContext(ctx);
  vm.runInContext(shim, ctx);
  const d = ctx.__tango;
  const down = () => posted.filter((s) => s.startsWith(PREFIX)).map((s) => JSON.parse(s.slice(PREFIX.length)));
  const reply = (i, status, name) => {
    const body = name ? Buffer.from(data.batches[name], "base64") : Buffer.alloc(0);
    const ab = body.buffer.slice(body.byteOffset, body.byteOffset + body.length);
    fetches[i].resolve({ ok: status >= 200 && status < 300, status, arrayBuffer: () => Promise.resolve(ab) });
  };
  const got = [];
  d.onMessage((m) => got.push(m));
  return { ctx, d, posted, fetches, down, reply, got, traceEvents };
}

const tick = () => new Promise((r) => setTimeout(r, 0));
const OPEN = { gen: 1, tok: "t0k", path: "/__tango/lane/poll" };

const scenarios = {
  "downshim: delivers batch in order": async () => {
    const h = setup();
    for (const k of ["_open", "_recvSeq", "_release", "_onPacket"]) assert.equal(typeof h.d[k], "function", k);
    assert.ok(Object.isFrozen(h.d));
    h.d._open(OPEN);
    await tick();
    assert.deepEqual(h.down(), [{ op: "poll", gen: 1 }], "poll op before the first fetch");
    assert.equal(h.fetches.length, 1);
    assert.equal(h.fetches[0].url, "/__tango/lane/poll?gen=1&tok=t0k&ack=0");
    h.reply(0, 200, "abc");
    await tick();
    assert.deepEqual(h.got, ["a", "b", "c"]);
    assert.equal(h.fetches.length, 2);
    assert.match(h.fetches[1].url, /[?&]ack=3$/);
    // An empty batch: poll again at once, ack unchanged.
    h.reply(1, 200, "empty4");
    await tick();
    assert.equal(h.fetches.length, 3);
    assert.match(h.fetches[2].url, /[?&]ack=3$/);
    // UTF-8 is decoded exactly.
    h.reply(2, 200, "utf8");
    await tick();
    assert.deepEqual(h.got, ["a", "b", "c", data.utf8]);
    assert.match(h.fetches[3].url, /[?&]ack=4$/);
    assert.deepEqual(h.down(), [{ op: "poll", gen: 1 }], "no other internal message");
    // _open again for the same generation does not start a second loop.
    h.d._open(OPEN);
    await tick();
    assert.equal(h.fetches.length, 4);
  },

  "downshim: trace records B poll timeline": async () => {
    const h = setup(true);
    h.d._open(OPEN);
    await tick();
    h.reply(0, 200, "abc");
    await tick();
    const names = h.traceEvents.map((e) => e.name);
    for (const name of ["down_mode", "poll_start", "next_fetch_issue", "fetch_response", "fetch_body", "decode_done", "downBatch_start", "downBatch_end"]) {
      assert.ok(names.includes(name), name);
    }
    assert.equal(h.traceEvents.find((e) => e.name === "down_mode").mode, "B");
    assert.deepEqual(h.traceEvents.find((e) => e.name === "decode_done"), {
      name: "decode_done", bytes: 30, first: 1, count: 3, items: 3, frames: 0,
    });
    assert.equal(h.traceEvents.find((e) => e.name === "next_fetch_issue").inflightMax, 1);
  },

  "downshim: reorders eval and poll": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.d._recvSeq(1, [[2, 0, "two"]]);
    assert.deepEqual(h.got, [], "seq 2 waits for seq 1");
    h.reply(0, 200, "one");
    await tick();
    assert.deepEqual(h.got, ["one", "two"]);
    assert.match(h.fetches[1].url, /[?&]ack=2$/);
  },

  "downshim: barrier pauses until release": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.reply(0, 200, "barrier7");
    await tick();
    assert.deepEqual(h.got, ["x"]);
    assert.deepEqual(h.down(), [{ op: "poll", gen: 1 }, { op: "barrier", gen: 1, id: 7 }]);
    assert.equal(h.fetches.length, 1, "no poll while paused");
    h.d._release(1, 8); // not the barrier waited on
    h.d._release(0, 7); // not this generation
    await tick();
    assert.deepEqual(h.got, ["x"]);
    h.d._release(1, 7);
    assert.deepEqual(h.got, ["x", "y"]);
    await tick();
    assert.equal(h.fetches.length, 2, "polling resumes after release");
    assert.match(h.fetches[1].url, /[?&]ack=3$/);
    h.d._release(1, 7); // twice is harmless
    assert.deepEqual(h.got, ["x", "y"]);
  },

  "downshim: barrier releases after throw": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.reply(0, 200, "barrier7");
    await tick();
    assert.deepEqual(h.got, ["x"]);
    // What the core runs for barrier 7: the script, releasing in finally.
    assert.throws(() => vm.runInContext(
      "try { throw new Error('script'); } finally { window.__tango._release(1, 7); }", h.ctx));
    assert.deepEqual(h.got, ["x", "y"]);
    await tick();
    assert.equal(h.fetches.length, 2);
  },

  "downshim: packets go to _onPacket": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.reply(0, 200, "pkts");
    await tick();
    assert.deepEqual(h.got, ["m"]);
    const pkts = [];
    const off = h.d._onPacket((p) => pkts.push(p));
    assert.deepEqual(pkts, [], "flush must be asynchronous");
    await tick();
    assert.equal(pkts.length, 2);
    for (const p of pkts) assert.equal(Object.prototype.toString.call(p), "[object Uint8Array]");
    assert.deepEqual(pkts.map((p) => Array.from(p)), [[1, 2, 3], [4, 5, 255]]);
    // Fallback packets come as base64 and go to the same subscriber.
    h.d._recvSeq(1, [[4, 1, Buffer.from([9, 8, 0, 200]).toString("base64")], [5, 0, "n"]]);
    assert.deepEqual(pkts.map((p) => Array.from(p)), [[1, 2, 3], [4, 5, 255], [9, 8, 0, 200]]);
    assert.deepEqual(h.got, ["m", "n"]);
    // A throwing subscriber does not stop the others.
    const offBad = h.d._onPacket(() => { throw new Error("pboom"); });
    const pkts2 = [];
    h.d._onPacket((p) => pkts2.push(p));
    h.d._recvSeq(1, [[6, 1, Buffer.from([7]).toString("base64")]]);
    assert.deepEqual(pkts2.map((p) => Array.from(p)), [[7]]);
    await tick();
    assert.deepEqual(thrown, ["pboom"]);
    offBad();
    off(); off();
    h.d._recvSeq(1, [[7, 1, Buffer.from([6]).toString("base64")]]);
    assert.equal(pkts.length, 4, "unsubscribed");
    assert.deepEqual(pkts2.map((p) => Array.from(p)), [[7], [6]]);
    assert.throws(() => h.d._onPacket("x"), (e) => e.name === "TypeError");
  },

  "downshim: fail reports acked": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.reply(0, 200, "abc");
    await tick();
    h.fetches[1].reject(new TypeError("Load failed"));
    await tick();
    assert.deepEqual(h.down(), [{ op: "poll", gen: 1 }, { op: "fail", gen: 1, acked: 3 }]);
    await tick();
    assert.equal(h.fetches.length, 2, "no fetch after failing");
    // The fallback keeps delivering in order; _open does not restart polling.
    h.d._recvSeq(1, [[5, 0, "e"], [4, 0, "d"]]);
    assert.deepEqual(h.got, ["a", "b", "c", "d", "e"]);
    h.d._open(OPEN);
    await tick();
    assert.equal(h.fetches.length, 2);
    assert.equal(h.down().length, 2, "fail is reported once");
  },

  "downshim: 403 fails": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.reply(0, 403);
    await tick();
    assert.deepEqual(h.down(), [{ op: "poll", gen: 1 }, { op: "fail", gen: 1, acked: 0 }]);
    assert.equal(h.fetches.length, 1);
  },

  "downshim: bad batch fails": async () => {
    for (const name of ["truncated", "trailing", "badkind", "shortbarrier"]) {
      const h = setup();
      h.d._open(OPEN);
      await tick();
      h.reply(0, 200, "one");
      await tick();
      h.reply(1, 200, name);
      await tick();
      assert.deepEqual(h.got, ["one"], name + ": nothing of a bad batch is delivered");
      assert.deepEqual(h.down(), [{ op: "poll", gen: 1 }, { op: "fail", gen: 1, acked: 1 }], name);
      assert.equal(h.fetches.length, 2, name);
    }
  },

  "downshim: 410 stops quietly": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.reply(0, 410);
    await tick();
    await tick();
    assert.deepEqual(h.down(), [{ op: "poll", gen: 1 }]);
    assert.equal(h.fetches.length, 1);
    // A new generation polls again.
    h.d._open({ gen: 2, tok: "n3w", path: "/__tango/lane/poll" });
    await tick();
    assert.equal(h.fetches.length, 2);
    assert.equal(h.fetches[1].url, "/__tango/lane/poll?gen=2&tok=n3w&ack=0");
  },

  "downshim: old generation ignored": async () => {
    const h = setup();
    h.d._open(OPEN);
    await tick();
    h.d._recvSeq(1, [[2, 0, "old2"]]);
    h.d._open({ gen: 2, tok: "n3w", path: "/__tango/lane/poll" });
    await tick();
    assert.equal(h.fetches.length, 2);
    assert.match(h.fetches[1].url, /gen=2&tok=n3w&ack=0$/);
    // gen 1's poll answering late is ignored; its loop does not go on.
    h.reply(0, 200, "abc");
    await tick();
    assert.deepEqual(h.got, []);
    assert.equal(h.fetches.length, 2);
    h.d._recvSeq(1, [[1, 0, "old1"]]);
    h.d._release(1, 7);
    h.d._open(OPEN);
    await tick();
    assert.deepEqual(h.got, []);
    assert.equal(h.fetches.length, 2);
    // gen 2 starts at 1; gen 1's buffered seq 2 was dropped.
    h.reply(1, 200, "one");
    await tick();
    assert.deepEqual(h.got, ["one"]);
    // A barrier of gen 2 is not released by gen 1.
    h.d._recvSeq(2, [[3, 0, "three"]]);
    h.reply(2, 200, "barrier9at2");
    await tick();
    assert.deepEqual(h.got, ["one"]);
    h.d._release(1, 9);
    assert.deepEqual(h.got, ["one"]);
    h.d._release(2, 9);
    assert.deepEqual(h.got, ["one", "three"]);
    await tick();
    assert.equal(h.fetches.length, 4, "gen 2 polls on after its release");
    // _recvSeq of a newer generation starts it without _open: gen 2's loop
    // stops and fallback alone delivers from seq 1.
    h.d._recvSeq(3, [[2, 0, "g3b"], [1, 0, "g3a"]]);
    assert.deepEqual(h.got, ["one", "three", "g3a", "g3b"]);
    h.reply(3, 200, "abc");
    await tick();
    assert.equal(h.fetches.length, 4, "gen 2 loop stopped");
    assert.deepEqual(h.got, ["one", "three", "g3a", "g3b"]);
    assert.ok(!h.down().some((m) => m.op === "fail"), "a stale generation never reports fail");
  },

  "downshim: no subscriber keeps queue": async () => {
    const ctx = { setTimeout, Promise, console, TextDecoder, atob };
    ctx.window = ctx;
    ctx.__tangoNative = { post() {} };
    const fetches = [];
    ctx.fetch = (url) => new Promise((resolve) => fetches.push(resolve));
    vm.createContext(ctx);
    vm.runInContext(shim, ctx);
    const d = ctx.__tango;
    d._open(OPEN);
    await tick();
    const body = Buffer.from(data.batches.abc, "base64");
    fetches[0]({ ok: true, status: 200, arrayBuffer: () => Promise.resolve(body.buffer.slice(body.byteOffset, body.byteOffset + body.length)) });
    await tick();
    d._recvSeq(1, [[4, 0, "d"]]);
    const got = [];
    d.onMessage((m) => got.push(m));
    assert.deepEqual(got, [], "flush must be asynchronous");
    await tick();
    assert.deepEqual(got, ["a", "b", "c", "d"]);
    assert.equal(fetches.length, 2, "polls on though nobody subscribed");
  },

  "downshim: duplicate seq dropped": async () => {
    const h = setup(true);
    h.d._open(OPEN);
    await tick();
    h.d._recvSeq(1, [[2, 0, "b"]]);
    h.d._recvSeq(1, [[2, 0, "B?"]]); // a duplicate of a waiting item
    h.reply(0, 200, "one");
    await tick();
    assert.deepEqual(h.got, ["one", "b"]);
    h.reply(1, 200, "abc"); // the poll sending from seq 1 again
    await tick();
    assert.deepEqual(h.got, ["one", "b", "c"]);
    assert.match(h.fetches[2].url, /[?&]ack=3$/);
    h.d._recvSeq(1, [[3, 0, "c"], [4, 0, "d"], [1, 0, "a"]]);
    assert.deepEqual(h.got, ["one", "b", "c", "d"]);
    h.reply(2, 200, "abc");
    await tick();
    assert.deepEqual(h.got, ["one", "b", "c", "d"]);
    assert.match(h.fetches[3].url, /[?&]ack=4$/);
    const duplicates = h.traceEvents.filter((e) => e.name === "down_duplicate");
    assert.ok(duplicates.length > 0, "duplicate sequences are traced");
    assert.ok(duplicates.every((e) => e.itemBytes > 0));
    assert.equal(duplicates.reduce((n, e) => n + e.packetBytes, 0), 0);
  },

  "downshim: fallback barrier not paused": async () => {
    const h = setup();
    // _recvSeq without _open: fallback only. Its barrier was run by the core.
    h.d._recvSeq(1, [[1, 0, "a"], [2, 2, 7], [3, 0, "b"]]);
    assert.deepEqual(h.got, ["a", "b"]);
    assert.deepEqual(h.down(), []);
    assert.equal(h.fetches.length, 0);
    // A fallback copy of the barrier waited on releases it (the core ran
    // the script inline).
    h.d._open({ gen: 2, tok: "t", path: "/p" });
    await tick();
    h.reply(0, 200, "barrier7");
    await tick();
    assert.deepEqual(h.got, ["a", "b", "x"]);
    h.d._recvSeq(2, [[2, 2, 7], [3, 0, "y"], [4, 0, "z"]]);
    assert.deepEqual(h.got, ["a", "b", "x", "y", "z"]);
    await tick();
    assert.equal(h.fetches.length, 2);
    assert.match(h.fetches[1].url, /^\/p\?gen=2&tok=t&ack=4$/);
    // A polled barrier still waiting behind a gap is replaced by the
    // fallback's copy: no pause, no barrier message.
    h.d._open({ gen: 3, tok: "t", path: "/p" });
    await tick();
    h.reply(2, 200, "barrier9at2");
    await tick();
    h.d._recvSeq(3, [[1, 0, "p"], [2, 2, 9], [3, 0, "q"]]);
    assert.deepEqual(h.got, ["a", "b", "x", "y", "z", "p", "q"]);
    assert.ok(!h.down().some((m) => m.op === "barrier" && m.gen === 3));
  },
};

(async () => {
  const fn = scenarios[process.argv[4]];
  if (!fn) throw new Error("no scenario " + process.argv[4]);
  await fn();
  console.log("ok");
})().catch((e) => { console.error(e); process.exit(1); });
`

func TestDownShimNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	msg := func(s string) downq.Item { return downq.Item{Kind: downq.Msg, Data: []byte(s)} }
	pkt := func(b ...byte) downq.Item { return downq.Item{Kind: downq.Packet, Data: b} }
	barrier := func(id uint64) downq.Item {
		return downq.Item{Kind: downq.Barrier, Data: binary.BigEndian.AppendUint64(nil, id)}
	}
	utf8 := "emoji \U0001F600 CJK 中文 line sep nul \x00"
	abc := downq.EncodeBatch(1, []downq.Item{msg("a"), msg("b"), msg("c")})
	batches := map[string][]byte{
		"abc":          abc,
		"one":          downq.EncodeBatch(1, []downq.Item{msg("one")}),
		"empty4":       downq.EncodeBatch(4, nil),
		"utf8":         downq.EncodeBatch(4, []downq.Item{msg(utf8)}),
		"barrier7":     downq.EncodeBatch(1, []downq.Item{msg("x"), barrier(7), msg("y")}),
		"barrier9at2":  downq.EncodeBatch(2, []downq.Item{barrier(9)}),
		"pkts":         downq.EncodeBatch(1, []downq.Item{pkt(1, 2, 3), msg("m"), pkt(4, 5, 255)}),
		"truncated":    abc[:len(abc)-1],
		"trailing":     append(append([]byte{}, abc...), 0),
		"badkind":      downq.EncodeBatch(2, []downq.Item{{Kind: 3, Data: []byte("q")}}),
		"shortbarrier": downq.EncodeBatch(2, []downq.Item{{Kind: downq.Barrier, Data: []byte{7}}}),
	}
	b64 := make(map[string]string, len(batches))
	for k, v := range batches {
		b64[k] = base64.StdEncoding.EncodeToString(v)
	}
	data, _ := json.Marshal(map[string]any{"batches": b64, "utf8": utf8})
	shimPath := write("shim.js", coreShim)
	dataPath := write("data.json", string(data))
	harness := write("harness.js", downHarness)
	for _, name := range []string{
		"downshim: trace records B poll timeline",
		"downshim: delivers batch in order",
		"downshim: reorders eval and poll",
		"downshim: barrier pauses until release",
		"downshim: barrier releases after throw",
		"downshim: packets go to _onPacket",
		"downshim: fail reports acked",
		"downshim: 403 fails",
		"downshim: bad batch fails",
		"downshim: 410 stops quietly",
		"downshim: old generation ignored",
		"downshim: no subscriber keeps queue",
		"downshim: duplicate seq dropped",
		"downshim: fallback barrier not paused",
	} {
		t.Run(name, func(t *testing.T) {
			out, err := exec.Command(node, harness, shimPath, dataPath, name).CombinedOutput()
			if err != nil || string(out) != "ok\n" {
				t.Fatalf("node: %v\n%s", err, out)
			}
		})
	}
}
