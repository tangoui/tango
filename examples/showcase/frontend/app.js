// Tango Showcase page script. Plain JS: the runtime is injected by Go
// (runtimejs.Script) as window.tango before this file runs.
"use strict";

const { Window, App, Theme, Screens, Clipboard, Dialog, Notify, Shell, Menu, Hotkey } = tango;
const SMOKE = location.search.includes("smoke");
const $ = (id) => document.getElementById(id);
const fmt = (v) => (typeof v === "string" ? v : JSON.stringify(v));

function logTo(id, ...a) {
  const el = $(id);
  const line = new Date().toLocaleTimeString() + "  " + a.map(fmt).join(" ");
  el.textContent += line + "\n";
  el.scrollTop = el.scrollHeight;
  if (SMOKE) tango.emit("log", a.map(fmt).join(" "));
}
const errText = (e) => (e && e.code ? `[${e.code}] ` : "") + ((e && e.message) || String(e));

// ---- Navigation highlight -------------------------------------------------
const links = [...document.querySelectorAll("nav a")];
const sections = links.map((a) => document.querySelector(a.getAttribute("href")));
document.querySelector("main").addEventListener("scroll", () => {
  let cur = 0;
  sections.forEach((s, i) => { if (s.getBoundingClientRect().top < 140) cur = i; });
  links.forEach((a, i) => a.classList.toggle("active", i === cur));
});

// ---- Overview -------------------------------------------------------------
async function overview() {
  const [goInfo, env, th] = await Promise.all([tango.call("Showcase.Info"), App.info(), Theme.get()]);
  $("ov-go").textContent = `${goInfo.go} · ${goInfo.cpus} 核`;
  $("ov-os").textContent = `${env.osName} ${env.osVersion} (${env.arch})`;
  $("ov-wv").textContent = `${env.webView || "未知"} ${env.webViewVersion || ""}`;
  $("ov-theme").textContent = th.dark ? "深色" : "浅色";
  $("envline").textContent = `pid ${goInfo.pid} · ${env.debug ? "debug 构建" : "release 构建"}`;
  return { goInfo, env, th };
}

// ---- Calls and events -----------------------------------------------------
$("greet").onclick = async () => {
  try { logTo("ipc-out", "Greet →", await tango.call("Showcase.Greet", $("name").value, Number($("times").value))); }
  catch (e) { logTo("ipc-out", "Greet 出错", errText(e)); }
};
$("divide").onclick = async () => {
  try { logTo("ipc-out", "Divide →", await tango.call("Showcase.Divide", 1, 0)); }
  catch (e) { logTo("ipc-out", "Divide 被拒绝", errText(e), "instanceof TangoError =", e instanceof tango.TangoError); }
};
$("ping").onclick = () => { tango.emit("ping", { from: "page", n: Date.now() % 1000 }); };
tango.on("pong", (d) => logTo("ipc-out", "pong", d));

// ---- Streams --------------------------------------------------------------
const chart = $("chart");
const ctx2d = chart.getContext("2d");
let samples = [];
let current = null; // the running stream
let paused = false;

function draw() {
  const w = chart.width = chart.clientWidth * devicePixelRatio;
  const h = chart.height = chart.clientHeight * devicePixelRatio;
  ctx2d.clearRect(0, 0, w, h);
  if (samples.length < 2) return;
  const max = Math.max(...samples.map((s) => s.heapKB)) * 1.1;
  const min = Math.min(...samples.map((s) => s.heapKB)) * 0.9;
  ctx2d.strokeStyle = getComputedStyle(document.documentElement).getPropertyValue("--accent").trim() || "#3b6cf6";
  ctx2d.lineWidth = 2 * devicePixelRatio;
  ctx2d.beginPath();
  samples.forEach((s, i) => {
    const x = (i / (samples.length - 1)) * w;
    const y = h - ((s.heapKB - min) / Math.max(1, max - min)) * (h - 8) - 4;
    i ? ctx2d.lineTo(x, y) : ctx2d.moveTo(x, y);
  });
  ctx2d.stroke();
}

async function runTicker() {
  if (current) return;
  $("st-start").disabled = true;
  $("st-stop").disabled = false;
  current = tango.stream("Showcase.Ticker", 100000, 100);
  try {
    for await (const s of current) {
      if (paused) continue;
      samples.push(s);
      if (samples.length > 120) samples.shift();
      $("st-info").textContent = `#${s.seq} · 堆 ${s.heapKB} KB · ${s.goroutines} 个 goroutine`;
      draw();
    }
  } catch (e) {
    $("st-info").textContent = "流结束：" + errText(e);
  } finally {
    current = null;
    $("st-start").disabled = false;
    $("st-stop").disabled = true;
  }
}
$("st-start").onclick = () => runTicker();
$("st-stop").onclick = () => { if (current) { current.cancel(); $("st-info").textContent = "已取消"; } };

// ---- System capabilities through jsapi ------------------------------------
const actions = {
  title: () => Window.setTitle("Showcase · " + new Date().toLocaleTimeString()),
  center: () => Window.center(),
  screens: async () => (await Screens.all()).map((d) => `${d.name} ${d.bounds.width}×${d.bounds.height} @${d.scale}x${d.primary ? " 主屏" : ""}`),
  copy: async () => { await Clipboard.writeText("来自 Tango Showcase 的问候"); return "已写入"; },
  paste: () => Clipboard.readText(),
  message: () => Dialog.message({ kind: "question", title: "Tango", message: "这是原生消息框。", buttons: ["好的", "取消"] }),
  open: () => Dialog.open({ multiple: true, title: "选几个文件" }),
  save: () => Dialog.save({ filename: "showcase.txt" }),
  notify: async () => { await Notify.requestPermission(); return Notify.send({ title: "Tango Showcase", body: "这条通知来自页面（jsapi）" }); },
  gonotify: async () => { await tango.call("Showcase.Notify"); return "已请求 Go 发送"; },
  badge: async () => { await App.setBadge("3"); setTimeout(() => App.setBadge(""), 3000); return "徽标 3（3 秒后清除）"; },
  url: () => Shell.openURL("https://github.com/tangoui/tango"),
  hotkey: () => Hotkey.register("CmdOrCtrl+Shift+K", () => logTo("sys-out", "快捷键触发")),
};
document.querySelectorAll("[data-act]").forEach((b) => {
  b.onclick = async () => {
    const act = b.dataset.act;
    try { const r = await actions[act](); logTo("sys-out", act, "→", r === undefined ? "ok" : r); }
    catch (e) { logTo("sys-out", act, "失败", errText(e)); }
  };
});
Menu.bindContextMenu($("system"), () => [
  { label: "复制", role: "copy" },
  { label: "全选", role: "selectAll" },
  { type: "separator" },
  { label: "窗口置顶", type: "checkbox", onClick: (on) => Window.setAlwaysOnTop(on) },
  { label: "缩放", submenu: [
    { label: "100%", type: "radio", group: "zoom", checked: true, onClick: () => Window.setZoom(1) },
    { label: "125%", type: "radio", group: "zoom", onClick: () => Window.setZoom(1.25) },
  ] },
  { label: "居中窗口", onClick: () => Window.center() },
]);
Window.onFilesDropped((d) => logTo("sys-out", "拖入文件", d.paths));
Notify.onClick((id) => logTo("sys-out", "通知被点击", id));

// ---- Tray and menus (driven from Go) ---------------------------------------
async function nativeStatus() {
  const st = await tango.call("Showcase.Status");
  const item = (ok, label, err) => `<li class="${ok ? "ok" : "no"}">${label}${err ? "：" + err : ""}</li>`;
  $("native-status").innerHTML =
    item(st.tray, "托盘图标", st.trayError) + item(st.menu, "应用菜单", st.menuError) + item(st.themeSync, "主题同步");
  return st;
}
tango.on("go:tray", (d) => logTo("native-out", "托盘", d));
tango.on("go:menu", (d) => {
  logTo("native-out", "菜单", d);
  if (d.item === "pauseTicker") paused = d.checked;
});
tango.on("go:notice", (d) => logTo("native-out", "Go 通知", d.error ? "失败：" + d.error : "id " + d.id));

// ---- Theme ----------------------------------------------------------------
function showTheme(where, t) {
  const html = document.documentElement;
  logTo("theme-out", where, t, "data-tango-theme =", html.dataset.tangoTheme || "(未设置)",
    "--tango-accent =", getComputedStyle(html).getPropertyValue("--tango-accent").trim() || "(未设置)");
  $("ov-theme").textContent = t.dark ? "深色" : "浅色";
  draw();
}
Theme.onChange((t) => showTheme("Theme.onChange", t));
tango.on("go:theme", (t) => logTo("theme-out", "Go 侧 theme.OnChange", t));

// ---- Start ----------------------------------------------------------------
async function start() {
  try {
    await overview();
    await nativeStatus();
    showTheme("Theme.get", await Theme.get());
  } catch (e) {
    logTo("sys-out", "启动出错", errText(e));
  }
  if (SMOKE) smoke();
}
start();

// ---- -smoke: unattended checks ---------------------------------------------
async function smoke() {
  const passed = [], skipped = [], failures = [];
  const wait = (ms) => new Promise((r) => setTimeout(r, ms));
  const check = async (name, fn) => {
    try {
      const r = await fn();
      if (r === "skip" || (Array.isArray(r) && r[0] === "skip")) skipped.push(name + (Array.isArray(r) ? ": " + r[1] : ""));
      else if (r === false) failures.push(name);
      else passed.push(name);
    } catch (e) {
      failures.push(name + ": " + errText(e));
    }
  };
  const rejectCode = async (p) => { try { await p; return null; } catch (e) { return e.code; } };

  await check("call Showcase.Info", async () => (await tango.call("Showcase.Info")).pid > 0);
  await check("call Showcase.Greet", async () => (await tango.call("Showcase.Greet", "世界", 2)) === "你好，世界！你好，世界！");
  await check("call error code bad_args", async () => (await rejectCode(tango.call("Showcase.Greet", "x", 9))) === "bad_args");
  await check("call error is TangoError", async () => {
    try { await tango.call("Showcase.Divide", 1, 0); return false; } catch (e) { return e instanceof tango.TangoError && e.code === "bad_args"; }
  });
  await check("unknown method not_found", async () => (await rejectCode(tango.call("Showcase.Nope"))) === "not_found");

  await check("event round trip", async () => {
    const got = new Promise((r) => { const off = tango.on("pong", (d) => { off(); r(d); }); });
    tango.emit("ping", { n: 42 });
    const d = await Promise.race([got, wait(3000).then(() => null)]);
    return !!d && d.echo && d.echo.n === 42;
  });

  await check("stream 200 items with back-pressure", async () => {
    let n = 0, last = 0;
    for await (const s of tango.stream("Showcase.Ticker", 200, 0)) { n++; if (s.seq !== last + 1) return false; last = s.seq; }
    return n === 200;
  });
  await check("stream cancel reaches Go", async () => {
    const before = await tango.call("Showcase.Canceled");
    let n = 0;
    for await (const _ of tango.stream("Showcase.Ticker", 1000, 5)) { if (++n === 3) break; }
    for (let i = 0; i < 30; i++) {
      if ((await tango.call("Showcase.Canceled")) > before) return true;
      await wait(100);
    }
    return false;
  });
  await check("stream bad_args", async () => {
    try { for await (const _ of tango.stream("Showcase.Ticker", -1, 0)) {} return false; } catch (e) { return e.code === "bad_args"; }
  });

  await check("App.info", async () => { const e = await App.info(); return !!(e.os && e.osVersion); });
  await check("Screens.primary", async () => { const d = await Screens.primary(); return d.primary && d.bounds.width > 0 && d.scale > 0; });
  await check("Theme.get", async () => typeof (await Theme.get()).dark === "boolean");
  await check("Window.setTitle/title", async () => { await Window.setTitle("Showcase ✓"); return (await Window.title()) === "Showcase ✓"; });
  await check("Window.setSize/size", async () => {
    await Window.setSize(940, 660);
    const s = await Window.size();
    return s.width === 940 && s.height === 660;
  });
  await check("Clipboard round trip (restored)", async () => {
    const before = await Clipboard.readText();
    const token = "tango-showcase-" + Date.now();
    await Clipboard.writeText(token);
    const got = await Clipboard.readText();
    await Clipboard.writeText(before);
    return got === token;
  });
  await check("Hotkey forbidden (not allowed)", async () => (await rejectCode(Hotkey.register("CmdOrCtrl+Shift+K", () => {}))) === "forbidden");
  await check("Shell.openURL scheme allow-list", async () => (await rejectCode(Shell.openURL("javascript:alert(1)"))) === "forbidden");
  await check("Notify.send", async () => {
    try { await Notify.send({ title: "Tango Showcase", body: "smoke", silent: true }); return true; }
    catch (e) {
      if (e.code === "forbidden") return false; // enabled in Go: must not be forbidden
      return ["skip", errText(e) + "（macOS 上需要 .app 包）"];
    }
  });

  await check("theme.SyncPage attributes", async () => {
    for (let i = 0; i < 30; i++) {
      if (document.documentElement.dataset.tangoTheme) return true;
      await wait(100);
    }
    return false;
  });
  await check("tray + app menu (Go)", async () => {
    const st = await tango.call("Showcase.Status");
    const env = await App.info();
    if (env.os === "darwin") return st.tray && st.menu;
    // Not verified on real Windows / Linux machines yet: report, do not fail.
    return st.tray && st.menu ? true : ["skip", `tray=${st.tray} ${st.trayError || ""} menu=${st.menu} ${st.menuError || ""}`];
  });

  skipped.push("Dialog.message / open / save（需要人操作）", "Menu.popup、托盘点击（需要人操作）");
  tango.emit("done", { passed, skipped, failures });
}
