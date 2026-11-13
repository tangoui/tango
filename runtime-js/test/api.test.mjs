// Runtime API modules (Window, App, ...) against a simulated Go side that
// answers "tango:*" calls the way the jsapi package does.
import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import * as tango from "../dist/index.js";

const { Window, App, Theme, Screens, Clipboard, Dialog, Notify, Shell, Menu, Hotkey, Events, TangoError } = tango;

// fakeGo installs window.__tango and plays Go: handlers maps method → fn
// (args) returning a result or throwing {code, message}.
function fakeGo(handlers = {}) {
  let sub = null;
  const go = {
    calls: [],
    events: [],
    post(s) {
      const m = JSON.parse(s);
      if (m.tango === "event") go.events.push(m);
      if (m.tango !== "call") return;
      go.calls.push({ method: m.method, args: m.args });
      const h = handlers[m.method];
      queueMicrotask(async () => {
        if (!h) return go.send({ tango: "ret", id: m.id, error: { code: "not_found", message: "method not found: " + m.method } });
        try {
          const result = await h(...m.args);
          go.send({ tango: "ret", id: m.id, result: result === undefined ? null : result });
        } catch (e) {
          go.send({ tango: "ret", id: m.id, error: { code: e.code ?? "internal", message: e.message ?? String(e) } });
        }
      });
    },
    onMessage(fn) {
      sub = fn;
      return () => (sub = null);
    },
    send(m) {
      sub(JSON.stringify(m));
    },
    emit(name, data) {
      go.send({ tango: "event", name, data });
    },
  };
  globalThis.__tango = go;
  return go;
}

const forbidden = (m) => () => {
  throw { code: "forbidden", message: m + " is not enabled" };
};

test("Window calls carry the right method names and arguments", async () => {
  const go = fakeGo({
    "tango:window.setTitle": () => null,
    "tango:window.size": () => ({ width: 640, height: 480 }),
    "tango:window.setSize": () => null,
    "tango:window.setSizeLimits": () => null,
    "tango:window.setPosition": () => null,
    "tango:window.state": () => "maximized",
    "tango:window.setAlwaysOnTop": () => null,
    "tango:window.id": () => 3,
  });
  await Window.setTitle("标题");
  assert.deepEqual(await Window.size(), { width: 640, height: 480 });
  await Window.setSize(800, 600);
  await Window.setSizeLimits({ minWidth: 300 });
  await Window.setPosition(10, 20);
  assert.equal(await Window.state(), "maximized");
  await Window.setAlwaysOnTop(true);
  assert.equal(await Window.id(), 3);
  assert.deepEqual(go.calls, [
    { method: "tango:window.setTitle", args: ["标题"] },
    { method: "tango:window.size", args: [] },
    { method: "tango:window.setSize", args: [800, 600] },
    { method: "tango:window.setSizeLimits", args: [300, 0, 0, 0] },
    { method: "tango:window.setPosition", args: [10, 20] },
    { method: "tango:window.state", args: [] },
    { method: "tango:window.setAlwaysOnTop", args: [true] },
    { method: "tango:window.id", args: [] },
  ]);
});

test("errors are TangoErrors with the Go code", async () => {
  fakeGo({
    "tango:clipboard.writeText": forbidden("tango:clipboard.writeText"),
    "tango:window.print": () => {
      throw { code: "unsupported", message: "not supported" };
    },
    "tango:dialog.open": () => {
      throw { code: "canceled", message: "canceled" };
    },
  });
  await assert.rejects(Clipboard.writeText("x"), (e) => e instanceof TangoError && e.code === "forbidden");
  await assert.rejects(Window.print(), (e) => e instanceof TangoError && e.code === "unsupported");
  await assert.rejects(Dialog.open({ multiple: true }), (e) => e.code === "canceled");
  await assert.rejects(Shell.openURL("https://x"), (e) => e.code === "not_found");
});

test("window events and unsubscribe", async () => {
  const go = fakeGo();
  const got = [];
  const off = Window.onResize((s) => got.push(["resize", s]));
  Window.onMove((p) => got.push(["move", p]));
  Window.onStateChange((s) => got.push(["state", s]));
  Window.onFilesDropped((d) => got.push(["drop", d]));
  go.emit("tango:window.resize", { width: 1, height: 2 });
  off();
  go.emit("tango:window.resize", { width: 3, height: 4 });
  go.emit("tango:window.move", { x: 5, y: 6 });
  go.emit("tango:window.state", { state: "minimized" });
  go.emit("tango:window.filesDropped", { paths: ["/a"], x: 0, y: 0 });
  go.emit("resize", { width: 9, height: 9 }); // app event of the same short name: not ours
  assert.deepEqual(got, [
    ["resize", { width: 1, height: 2 }],
    ["move", { x: 5, y: 6 }],
    ["state", "minimized"],
    ["drop", { paths: ["/a"], x: 0, y: 0 }],
  ]);
});

test("focus and blur come from the window system", async () => {
  const go = fakeGo({ "tango:window.isFocused": () => true, "tango:window.title": () => "T" });
  let n = 0;
  const off = Window.onFocus(() => n++);
  Window.onBlur(() => (n += 10));
  go.emit("tango:window.focus", null);
  go.emit("tango:window.blur", null);
  off();
  go.emit("tango:window.focus", null);
  assert.equal(n, 11);
  assert.equal(await Window.isFocused(), true);
  assert.equal(await Window.title(), "T");
});

test("App launch events deliver pending ones once", async () => {
  const pending = { openURL: [{ url: "demo://launch" }], openFiles: [{ paths: ["/f"] }] };
  const go = fakeGo({
    "tango:app.takePending": (kind) => {
      const l = pending[kind] ?? [];
      pending[kind] = [];
      return l;
    },
    "tango:env.info": () => ({ os: "darwin", arch: "arm64", osName: "macOS", osVersion: "15", debug: true, webView: "WebKit", webViewVersion: "1" }),
  });
  const urls = [];
  App.onOpenURL((u) => urls.push(u));
  App.onOpenURL((u) => urls.push("second:" + u)); // only the first subscriber gets pending ones
  const files = [];
  App.onOpenFiles((p) => files.push(p));
  await new Promise((r) => setTimeout(r, 5));
  go.emit("tango:app.openURL", { url: "demo://live" });
  assert.deepEqual(urls, ["demo://launch", "demo://live", "second:demo://live"]);
  assert.deepEqual(files, [["/f"]]);
  assert.equal(go.calls.filter((c) => c.method === "tango:app.takePending").length, 2);
  const si = [];
  App.onSecondInstance((d) => si.push(d));
  go.emit("tango:app.secondInstance", { args: ["x"], cwd: "/" });
  assert.deepEqual(si, [{ args: ["x"], cwd: "/" }]);
  assert.equal((await App.info()).osName, "macOS");
});

test("Theme, Screens and Notify events", async () => {
  const go = fakeGo({
    "tango:theme.get": () => ({ dark: false, accent: "" }),
    "tango:screens.all": () => [{ id: "1", primary: true }],
    "tango:notify.send": (n) => "n-" + n.title,
  });
  assert.deepEqual(await Theme.get(), { dark: false, accent: "" });
  assert.equal((await Screens.all()).length, 1);
  const seen = [];
  Theme.onChange((t) => seen.push(t.dark));
  Screens.onChange((d) => seen.push(d.length));
  Notify.onClick((id) => seen.push(id));
  go.emit("tango:theme.change", { dark: true, accent: "#000000ff" });
  go.emit("tango:screens.change", [{}, {}]);
  assert.equal(await Notify.send({ title: "t" }), "n-t");
  go.emit("tango:notify.click", { id: "n-t" });
  assert.deepEqual(seen, [true, 2, "n-t"]);
});

test("Theme.vars and onVarsChange read the injected design tokens", () => {
  assert.deepEqual(Theme.vars(), {});
  const target = new EventTarget();
  globalThis.addEventListener = target.addEventListener.bind(target);
  globalThis.removeEventListener = target.removeEventListener.bind(target);
  try {
    const seen = [];
    const off = Theme.onVarsChange((v) => seen.push(v));
    globalThis.__tangoUIVarsState = { "--tango-color-primary": "#1677ff" };
    target.dispatchEvent(new CustomEvent("tangouivars", { detail: { "--tango-color-primary": "#1677ff" } }));
    off();
    target.dispatchEvent(new CustomEvent("tangouivars", { detail: {} }));
    assert.deepEqual(seen, [{ "--tango-color-primary": "#1677ff" }]);
    assert.deepEqual(Theme.vars(), { "--tango-color-primary": "#1677ff" });
  } finally {
    delete globalThis.addEventListener;
    delete globalThis.removeEventListener;
    delete globalThis.__tangoUIVarsState;
  }
});

test("Clipboard image strips a data URL prefix", async () => {
  const go = fakeGo({ "tango:clipboard.writeImage": () => null, "tango:clipboard.readImage": () => null, "tango:clipboard.writeFiles": () => null });
  await Clipboard.writeImage("data:image/png;base64,QUJD");
  await Clipboard.writeImage("QUJD");
  assert.equal(await Clipboard.readImage(), null);
  assert.deepEqual(go.calls.slice(0, 2).map((c) => c.args[0]), ["QUJD", "QUJD"]);
  await Clipboard.writeFiles(["/a"]);
  assert.deepEqual(go.calls.at(-1), { method: "tango:clipboard.writeFiles", args: [["/a"]] });
});

test("Menu.popup sends plain items and runs the chosen onClick", async () => {
  let wire;
  fakeGo({
    "tango:menu.popup": (items, x, y) => {
      wire = { items, x, y };
      return { id: "1.0", checked: false };
    },
  });
  const clicked = [];
  const r = await Menu.popup(
    [
      { label: "Open", onClick: () => clicked.push("open") },
      {
        label: "More",
        submenu: [{ label: "Deep", onClick: () => clicked.push("deep") }],
      },
      { type: "separator" },
      { role: "copy" },
    ],
    { x: 10.6, y: 20.2 },
  );
  assert.deepEqual(r, { id: "1.0", checked: false });
  assert.deepEqual(clicked, ["deep"]);
  assert.equal(wire.x, 11);
  assert.equal(wire.y, 20);
  assert.deepEqual(wire.items, [
    { label: "Open", id: "0" },
    { label: "More", id: "1", submenu: [{ label: "Deep", id: "1.0" }] },
    { type: "separator", id: "2" },
    { role: "copy", id: "3" },
  ]);
});

test("Menu.popup resolves null when dismissed; bindContextMenu", async () => {
  fakeGo({ "tango:menu.popup": () => ({ id: null, checked: false }) });
  assert.equal(await Menu.popup([{ label: "x" }], { x: 0, y: 0 }), null);
  const ls = new Map();
  const target = { addEventListener: (t, f) => ls.set(t, f), removeEventListener: (t) => ls.delete(t) };
  const off = Menu.bindContextMenu(target, (ev) => (ev.skip ? null : [{ label: "a" }]));
  let prevented = 0;
  ls.get("contextmenu")({ clientX: 1, clientY: 2, preventDefault: () => prevented++ });
  ls.get("contextmenu")({ skip: true, preventDefault: () => prevented++ });
  assert.equal(prevented, 1);
  off();
  assert.equal(ls.size, 0);
});

test("Hotkey.register routes fire events by id and unregisters", async () => {
  let next = 0;
  const go = fakeGo({ "tango:hotkey.register": () => ++next, "tango:hotkey.unregister": () => null });
  const fired = [];
  const offA = await Hotkey.register("CmdOrCtrl+K", () => fired.push("a"));
  await Hotkey.register("CmdOrCtrl+J", () => fired.push("b"));
  go.emit("tango:hotkey.fire", { id: 2, accelerator: "CmdOrCtrl+J" });
  go.emit("tango:hotkey.fire", { id: 1, accelerator: "CmdOrCtrl+K" });
  await offA();
  await offA(); // second call is a no-op
  go.emit("tango:hotkey.fire", { id: 1 });
  assert.deepEqual(fired, ["b", "a"]);
  assert.deepEqual(go.calls.filter((c) => c.method === "tango:hotkey.unregister"), [{ method: "tango:hotkey.unregister", args: [1] }]);
});

test("Events: on, once, next, emit", async () => {
  const go = fakeGo();
  const got = [];
  Events.once("tick", (d) => got.push(d));
  const p = Events.next("tick");
  go.emit("tick", 1);
  go.emit("tick", 2);
  assert.equal(await p, 1);
  assert.deepEqual(got, [1]);
  Events.emit("hi", { a: 1 });
  assert.deepEqual(go.events, [{ tango: "event", name: "hi", data: { a: 1 } }]);
});

test("module functions reject outside Tango", async () => {
  delete globalThis.__tango;
  await assert.rejects(Window.setTitle("x"), (e) => e instanceof TangoError && e.code === "not_found");
});

test("IIFE exposes the modules on window.tango", () => {
  const src = readFileSync(new URL("../runtimejs/tango.iife.js", import.meta.url), "utf8");
  const ctx = { setTimeout, queueMicrotask };
  ctx.globalThis = ctx;
  vm.runInNewContext(src, ctx);
  for (const ns of ["Window", "App", "Theme", "Screens", "Clipboard", "Dialog", "Notify", "Shell", "Menu", "Hotkey", "Events"]) {
    assert.equal(typeof ctx.tango[ns], "object", ns);
  }
  assert.equal(typeof ctx.tango.Window.setTitle, "function");
  assert.equal(typeof ctx.tango.Dialog.message, "function");
});

test("Web calls and download events", async () => {
  const { Web } = tango;
  const go = fakeGo({
    "tango:web.find": () => ({ matches: 3, active: 1, found: true }),
    "tango:web.findNext": () => ({ matches: 3, active: 2, found: true }),
    "tango:web.stopFind": () => null,
    "tango:web.screenshot": () => "iVBORw0KGgo=",
    "tango:web.cookies": () => [{ name: "a", value: "1", domain: "x.test", path: "/", expires: null, secure: false, httpOnly: false, sameSite: "" }],
    "tango:web.clearData": () => null,
    "tango:web.setCookie": forbidden("tango:web.setCookie"),
  });
  assert.deepEqual(await Web.find("apple", { matchCase: true }), { matches: 3, active: 1, found: true });
  assert.equal((await Web.findNext()).active, 2);
  await Web.stopFind();
  assert.equal(await Web.screenshot(), "iVBORw0KGgo=");
  assert.equal((await Web.cookies("https://x.test/"))[0].name, "a");
  await Web.cookies();
  await Web.clearData(["cache"]);
  await Web.clearData();
  await assert.rejects(Web.setCookie({ name: "a", value: "1", domain: "x.test" }), (e) => e.code === "forbidden");
  const seen = [];
  const off = Web.onDownload((d) => seen.push(d));
  go.emit("tango:web.download", { id: 1, state: "done", received: 4, total: 4, path: "/tmp/a" });
  off();
  go.emit("tango:web.download", { id: 2, state: "done", received: 4, total: 4, path: "/tmp/b" });
  await new Promise((r) => setTimeout(r, 0));
  assert.deepEqual(seen.map((d) => d.id), [1]);
  assert.deepEqual(
    go.calls.map((c) => [c.method, c.args]),
    [
      ["tango:web.find", ["apple", { matchCase: true }]],
      ["tango:web.findNext", []],
      ["tango:web.stopFind", []],
      ["tango:web.screenshot", []],
      ["tango:web.cookies", ["https://x.test/"]],
      ["tango:web.cookies", []],
      ["tango:web.clearData", [["cache"]]],
      ["tango:web.clearData", []],
      ["tango:web.setCookie", [{ name: "a", value: "1", domain: "x.test" }]],
    ],
  );
});
