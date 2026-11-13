// E2E harness: runs the embedded IIFE in a VM context whose window.__tango is
// wired to stdio. Each line on stdin/stdout is one raw message encoded as a
// JSON string literal. Usage: node harness.mjs <tango.iife.js>
import { readFileSync } from "node:fs";
import { createInterface } from "node:readline";
import vm from "node:vm";

const out = (msg) => process.stdout.write(JSON.stringify(msg) + "\n");

// Minimal core shim: buffer Go→JS messages until the first subscriber.
const subs = new Set();
let pending = [];
const bridge = {
  post: (s) => out(String(s)),
  onMessage(fn) {
    subs.add(fn);
    if (pending) {
      const p = pending;
      pending = null;
      for (const m of p) fn(m);
    }
    return () => subs.delete(fn);
  },
};
createInterface({ input: process.stdin }).on("line", (line) => {
  const msg = JSON.parse(line);
  if (pending) pending.push(msg);
  else for (const fn of [...subs]) fn(msg);
});

const ctx = vm.createContext({ __tango: bridge, setTimeout, clearTimeout });
ctx.window = ctx;
vm.runInContext(readFileSync(process.argv[2], "utf8"), ctx);
const tango = ctx.tango;

const once = (name) => new Promise((resolve) => {
  const off = tango.on(name, (d) => {
    off();
    resolve(d);
  });
});

async function scenario() {
  const r = {};
  const news = new Promise((resolve) => {
    tango.onStream("news", async (s, topic) => {
      const items = [];
      for await (const v of s) items.push(v);
      resolve({ topic, count: items.length, last: items.at(-1) });
    });
  });

  r.hello = await tango.call("Greeter.Hello", "world");
  r.div = await tango.call("Svc.Div", 9, 3);
  try {
    await tango.call("Svc.Div", 1, 0);
  } catch (e) {
    r.divErr = e.code;
  }
  try {
    await tango.call("Svc.Div", "x");
  } catch (e) {
    r.badArgs = e.code;
  }
  try {
    await tango.call("Nope");
  } catch (e) {
    r.missing = e.code;
  }

  let sum = 0;
  let n = 0;
  for await (const v of tango.stream("Count", 300)) {
    sum += v;
    n++;
  }
  r.count = { n, sum };

  const canceled = once("canceled");
  let taken = 0;
  for await (const _ of tango.stream("Forever")) if (++taken === 10) break;
  r.canceled = await canceled;

  const pong = once("pong");
  tango.emit("ping", { n: 41 });
  r.pong = await pong;

  tango.emit("open-news");
  r.news = await news;

  const echo = new Promise((resolve) => tango.onRaw(resolve));
  tango.postRaw("raw hello");
  r.raw = await echo;
  return r;
}

scenario().then(
  (r) => {
    out("DONE " + JSON.stringify(r));
  },
  (e) => {
    out("FAIL " + (e && e.stack ? e.stack : String(e)));
  },
);
