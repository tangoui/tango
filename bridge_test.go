package tango

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// shimHarness exercises coreShim in node. argv[2] is the shim file,
// argv[3] a JSON file {"msgs": [...], "evals": [...]} where evals[i] is
// recvJS(msgs[i]) produced by Go.
const shimHarness = `
const assert = require("node:assert/strict");
const fs = require("node:fs");
const vm = require("node:vm");
const shim = fs.readFileSync(process.argv[2], "utf8");
const data = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));

const thrown = [];
process.on("uncaughtException", (e) => thrown.push(e.message));

const posted = [];
const ctx = { setTimeout, Promise, console };
ctx.window = ctx;
ctx.__tangoNative = { post(s) { posted.push(s); } };
vm.createContext(ctx);
vm.runInContext(shim, ctx);
const d = ctx.__tango;

(async () => {
  // post goes to the native bridge; only strings.
  d.post("hi");
  assert.deepEqual(posted, ["hi"]);
  assert.throws(() => d.post(42), (e) => e.name === "TypeError");
  assert.throws(() => d.onMessage("x"), (e) => e.name === "TypeError");

  // frozen, and running the shim again keeps the same object.
  assert.ok(Object.isFrozen(d));
  vm.runInContext(shim, ctx);
  assert.equal(ctx.__tango, d);

  // Messages before any subscriber are kept and flushed in order to the
  // first subscriber, asynchronously; ones arriving meanwhile queue behind.
  d._recv("a");
  d._recv("b");
  const got = [];
  const off = d.onMessage((m) => got.push(m));
  assert.deepEqual(got, [], "flush must be asynchronous");
  d._recv("c");
  assert.deepEqual(got, [], "must queue behind pending");
  await Promise.resolve();
  await Promise.resolve();
  assert.deepEqual(got, ["a", "b", "c"]);

  // Delivered synchronously once subscribed; to every subscriber.
  const got2 = [];
  const off2 = d.onMessage((m) => got2.push(m));
  d._recv("d");
  assert.deepEqual(got, ["a", "b", "c", "d"]);
  assert.deepEqual(got2, ["d"]);

  // A throwing subscriber does not stop the others; error rethrown later.
  const offBad = d.onMessage(() => { throw new Error("boom"); });
  const got3 = [];
  const off3 = d.onMessage((m) => got3.push(m));
  d._recv("e");
  assert.deepEqual(got3, ["e"]);
  await new Promise((r) => setTimeout(r, 10));
  assert.deepEqual(thrown, ["boom"]);
  offBad();

  // Unsubscribe; unsubscribing twice is harmless.
  off(); off();
  off2();
  d._recv("f");
  assert.deepEqual(got, ["a", "b", "c", "d", "e"]);
  assert.deepEqual(got3, ["e", "f"]);
  off3();

  // With nobody subscribed messages are kept again.
  d._recv("g");
  const got4 = [];
  d.onMessage((m) => got4.push(m));
  await Promise.resolve();
  assert.deepEqual(got4, ["g"]);

  // Go's recvJS output round-trips every message exactly.
  const recv = [];
  d.onMessage((m) => recv.push(m));
  for (const js of data.evals) vm.runInContext(js, ctx);
  assert.deepEqual(recv, data.msgs);

  // Without __tangoNative, post throws (no silent loss).
  const ctx2 = { setTimeout, Promise };
  ctx2.window = ctx2;
  vm.createContext(ctx2);
  vm.runInContext(shim, ctx2);
  assert.throws(() => ctx2.__tango.post("x"));

  console.log("ok");
})().catch((e) => { console.error(e); process.exit(1); });
`

func TestCoreShimNode(t *testing.T) {
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
	ls, ps := string(rune(0x2028)), string(rune(0x2029))
	msgs := []string{
		"plain",
		"",
		`{"tango":"event","name":"x","data":{"pct":42}}`,
		"quote \" backslash \\ newline \n tab \t",
		"line" + ls + "para" + ps + "end",
		"</script><script>alert(1)</script>",
		"emoji \U0001F600 and CJK 中文",
		"nul \x00 ctrl \x1f",
	}
	evals := make([]string, len(msgs))
	for i, m := range msgs {
		evals[i] = recvJS(m)
	}
	data, _ := json.Marshal(map[string]any{"msgs": msgs, "evals": evals})
	shimPath := write("shim.js", coreShim)
	dataPath := write("data.json", string(data))
	harness := write("harness.js", shimHarness)
	out, err := exec.Command(node, harness, shimPath, dataPath).CombinedOutput()
	if err != nil || string(out) != "ok\n" {
		t.Fatalf("node: %v\n%s", err, out)
	}
}
