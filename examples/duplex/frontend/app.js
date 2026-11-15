// The duplex example page: Echo, Upload, a terminal and the benchmark, all
// over window.tango streams (the runtime-js IIFE injected by the app).
"use strict";

window.addEventListener("error", (e) => window.__tango && __tango.post("page-error: " + e.message));
window.addEventListener("unhandledrejection", (e) =>
  window.__tango && __tango.post("page-reject: " + ((e.reason && e.reason.message) || e.reason)));

const $ = (id) => document.getElementById(id);
const logTo = (id) => (s) => {
  const out = $(id);
  out.textContent += s + "\n";
  out.scrollTop = out.scrollHeight;
};
const hex = (b) => Array.from(b, (x) => x.toString(16).padStart(2, "0")).join(" ");

// ---------------------------------------------------------------------------
// Echo: one long-lived socket for the buttons.

const echoLog = logTo("echo-out");
let echoSocket = null;

function echoSock() {
  if (echoSocket && echoSocket.readyState <= 1) return echoSocket;
  const ws = tango.socket("Echo");
  ws.onopen = () => echoLog("open");
  ws.onmessage = (e) => {
    if (e.data instanceof ArrayBuffer) echoLog("← bytes: " + hex(new Uint8Array(e.data)));
    else echoLog("← json:  " + e.data);
  };
  ws.onclose = (e) => echoLog("close " + e.code);
  echoSocket = ws;
  return ws;
}

$("echo-json").onclick = () => {
  const v = $("echo-text").value;
  echoLog("→ json:  " + JSON.stringify(v));
  echoSock().send(v);
};
$("echo-bytes").onclick = () => {
  const b = crypto.getRandomValues(new Uint8Array(8));
  echoLog("→ bytes: " + hex(b));
  echoSock().send(b);
};

/** Sends v once through a fresh Echo stream and resolves with the echo. */
async function echoOnce(v) {
  const s = tango.stream("Echo");
  s.send(v);
  s.end();
  let got;
  for await (const m of s) got = m;
  return got;
}

// ---------------------------------------------------------------------------
// Upload: blob.stream() piped into the stream's writable; progress and the
// final result come back on the same stream.

const uploadLog = logTo("upload-out");

/** Uploads blob (a File or Blob) and resolves with {path, bytes}. */
async function upload(blob, name) {
  const bar = $("upload-progress");
  bar.max = Math.max(blob.size, 1);
  bar.value = 0;
  const s = tango.stream("Upload", name, blob.size);
  const replies = (async () => {
    let done = null;
    for await (const m of s) {
      if (m && typeof m.received === "number") bar.value = m.received;
      if (m && typeof m.path === "string") done = m;
    }
    if (!done) throw new Error("upload ended without a result");
    return done;
  })();
  await blob.stream().pipeTo(s.writable);
  const done = await replies;
  bar.value = bar.max;
  uploadLog(`${name}: ${done.bytes} 字节 → ${done.path}`);
  return done;
}

$("upload-file").onchange = async (e) => {
  const f = e.target.files[0];
  if (!f) return;
  try {
    await upload(f, f.name);
  } catch (err) {
    uploadLog("失败: " + err.message);
  }
};

// ---------------------------------------------------------------------------
// Terminal: Go's "Term" shell. Its output arrives as byte frames on
// stream.readable, decoded as UTF-8 text; each input line goes to
// stream.writable as bytes.

const termOut = $("term-out");
let term = null; // {stream, writer} of the running shell

function termPrint(text) {
  termOut.textContent += text;
  termOut.scrollTop = termOut.scrollHeight;
}

function startTerm() {
  const stream = tango.stream("Term");
  const writer = stream.writable.getWriter();
  const me = { stream, writer };
  term = me;
  (async () => {
    const reader = stream.readable.pipeThrough(new TextDecoderStream()).getReader();
    try {
      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        termPrint(value);
      }
      termPrint("\n[shell 已退出]\n");
    } catch (err) {
      if (term === me) termPrint("\n[" + err.message + "]\n"); // not after a restart
    }
    if (term === me) term = null;
  })();
}

const utf8 = new TextEncoder();

$("term-in").onkeydown = (e) => {
  if (e.key !== "Enter" || e.isComposing) return;
  const line = e.target.value;
  e.target.value = "";
  if (!term) startTerm();
  term.writer.write(utf8.encode(line + "\n")).catch((err) => termPrint("\n[" + err.message + "]\n"));
};

$("term-restart").onclick = () => {
  const old = term;
  term = null;
  if (old) old.stream.cancel(); // Go kills the shell when the stream ends
  termOut.textContent = "";
  startTerm();
};

if (!location.search.includes("bench")) startTerm();

// ---------------------------------------------------------------------------
// Benchmark.

const benchLog = logTo("bench-out");

/** The benchmark of /bench.js (window.duplexBench, shared with
 * examples/duplex-mobile); the result object of docs/ipc-bench.md. */
const bench = () => duplexBench.run(benchLog);

async function runBench() {
  $("bench-run").disabled = true;
  try {
    const res = await bench();
    benchLog(JSON.stringify(res));
    await tango.call("Bench.Report", res);
    return res;
  } catch (err) {
    benchLog("失败: " + err.message);
    if (location.search.includes("bench")) await tango.call("Bench.Report", { error: String(err.message || err) });
    throw err;
  } finally {
    $("bench-run").disabled = false;
  }
}

$("bench-run").onclick = () => runBench().catch(() => {});

if (location.search.includes("panel")) document.body.classList.add("with-panel");

window.duplex = { echoOnce, upload, bench, runBench };

if (location.search.includes("bench")) runBench().catch(() => {});
