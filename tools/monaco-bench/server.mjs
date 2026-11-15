// Local static server for the Monaco baseline (tools/monaco-bench).
//
// Serves, on 127.0.0.1 only (random port unless --port):
//   /                      page.html (this directory)
//   /monaco/package.json   the monaco-editor package.json (version check)
//   /monaco/vs/...         <monaco>/min/vs/... (the AMD build: loader.js + editor.main)
//   /data/<name>           files of the data directory (the Tango perf generator's
//                          cache, $TMPDIR/tango-editor-perf), plain names only
//
// No dependencies; nothing leaves the machine. Run standalone for manual use:
//   node tools/monaco-bench/server.mjs --monaco <monaco-editor dir> [--data <dir>] [--port N]

import http from 'node:http';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));

export const defaultDataDir = path.join(os.tmpdir(), 'tango-editor-perf');

const types = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.ttf': 'font/ttf',
  '.ts': 'text/plain; charset=utf-8',
};

// within reports whether p is root or inside it.
function within(root, p) {
  const rel = path.relative(root, p);
  return rel === '' || (!rel.startsWith('..') && !path.isAbsolute(rel));
}

function sendFile(res, file) {
  fs.stat(file, (err, st) => {
    if (err || !st.isFile()) {
      res.writeHead(404, { 'content-type': 'text/plain' });
      res.end('not found');
      return;
    }
    res.writeHead(200, {
      'content-type': types[path.extname(file)] || 'application/octet-stream',
      'content-length': st.size,
      'cache-control': 'no-store',
      // cross-origin isolation: performance.now() at 5 µs instead of 100 µs
      'cross-origin-opener-policy': 'same-origin',
      'cross-origin-embedder-policy': 'require-corp',
      'cross-origin-resource-policy': 'same-origin',
    });
    fs.createReadStream(file).pipe(res);
  });
}

// startServer serves page.html, the Monaco build and the data files; it
// resolves to { server, port, url, close() }.
export function startServer({ monacoDir, dataDir = defaultDataDir, port = 0 }) {
  const monacoRoot = path.resolve(monacoDir);
  const vsRoot = path.join(monacoRoot, 'min', 'vs');
  if (!fs.existsSync(path.join(vsRoot, 'loader.js'))) {
    throw new Error(`no ${path.join(vsRoot, 'loader.js')}: --monaco must be a monaco-editor package directory`);
  }
  const dataRoot = path.resolve(dataDir);
  const server = http.createServer((req, res) => {
    let u;
    try {
      u = new URL(req.url, 'http://127.0.0.1');
    } catch {
      res.writeHead(400);
      res.end();
      return;
    }
    const p = decodeURIComponent(u.pathname);
    if (req.method !== 'GET' && req.method !== 'HEAD') {
      res.writeHead(405);
      res.end();
      return;
    }
    if (p === '/' || p === '/page.html') return sendFile(res, path.join(here, 'page.html'));
    if (p === '/favicon.ico') {
      res.writeHead(204);
      res.end();
      return;
    }
    if (p === '/monaco/package.json') return sendFile(res, path.join(monacoRoot, 'package.json'));
    if (p.startsWith('/monaco/vs/')) {
      const f = path.join(vsRoot, p.slice('/monaco/vs/'.length));
      if (within(vsRoot, f)) return sendFile(res, f);
    }
    if (p.startsWith('/data/')) {
      const name = p.slice('/data/'.length);
      if (/^[A-Za-z0-9._-]+$/.test(name)) return sendFile(res, path.join(dataRoot, name));
    }
    res.writeHead(404, { 'content-type': 'text/plain' });
    res.end('not found');
  });
  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(port, '127.0.0.1', () => {
      const actual = server.address().port;
      resolve({
        server,
        port: actual,
        url: `http://127.0.0.1:${actual}/`,
        close: () => new Promise((r) => { server.closeAllConnections?.(); server.close(() => r()); }),
      });
    });
  });
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const args = process.argv.slice(2);
  const get = (k, d) => { const i = args.indexOf(k); return i >= 0 ? args[i + 1] : d; };
  const monacoDir = get('--monaco');
  if (!monacoDir) {
    console.error('usage: node server.mjs --monaco <monaco-editor dir> [--data <dir>] [--port N]');
    process.exit(2);
  }
  const s = await startServer({ monacoDir, dataDir: get('--data', defaultDataDir), port: Number(get('--port', 0)) });
  console.log(`serving ${s.url} (Ctrl-C to stop)`);
  process.on('SIGINT', async () => { await s.close(); process.exit(0); });
}
