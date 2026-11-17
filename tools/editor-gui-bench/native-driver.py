#!/usr/bin/env python3
"""Tango / MyGo native-editor driver for the GUI acceptance probe.

  native-driver.py --engine tango|mygo --bin <dir> --out <dir>
                   [--scenarios type,multi,scroll] [--keys 100] [--multi-keys 30]
                   [--scrolls 240] [--templates-from <dir>]

<dir> given to --bin holds gui-probe, tango-fixture (built with -tags
tango_guiprobe) and mygo-fixture. Per scenario: a record pass (baseline wait
plus inputs, each recording caret-free settled crops), templates
(make-templates.mjs), the same initial state again, then a measure pass that
aligns to the record baseline and matches the templates; latency bounds come
from latency.mjs and the final text is checked exactly. With
--templates-from the record pass of an earlier run is reused (same regions
required), so a matrix records once per engine. The capture is of the target
window (isolated): WindowServer's composited window, not the display.
"""
import argparse, json, math, pathlib, shutil, subprocess, sys, time, urllib.request

HERE = pathlib.Path(__file__).resolve().parent
SCALE = 2

ap = argparse.ArgumentParser()
ap.add_argument('--engine', choices=['tango', 'mygo'], required=True)
ap.add_argument('--bin', required=True)
ap.add_argument('--out', required=True)
ap.add_argument('--scenarios', default='type,multi,scroll')
ap.add_argument('--keys', type=int, default=10)
ap.add_argument('--multi-keys', type=int, default=10)
ap.add_argument('--scrolls', type=int, default=30)
ap.add_argument('--templates-from')
opt = ap.parse_args()
BIN, OUT = pathlib.Path(opt.bin).resolve(), pathlib.Path(opt.out).resolve()
FIXTURE = {'tango': 'tango-fixture', 'mygo': 'mygo-fixture'}[opt.engine]
NEEDS_CLICK = opt.engine == 'tango'  # MyGo focuses its editor with AutoFocus
OUT.mkdir(parents=True, exist_ok=True)


def call(cfg, path, body={}):
    req = urllib.request.Request(cfg['url'] + path, data=json.dumps(body).encode(), method='POST',
                                 headers={'Authorization': 'Bearer ' + cfg['token'], 'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=8) as r:
        return json.loads(r.read())


def node(*a):
    p = subprocess.run(['node', *map(str, a)], capture_output=True, text=True)
    return p.returncode, (p.stdout.strip().splitlines() or [''])[-1], p.stderr[-300:]


def probe_bg(d, name, cfg, w, acts, extra=None):
    job = {'pid': cfg['pid'], 'windowID': w['windowID'], 'isolatedWindow': True,
           'inputSource': 'com.apple.keylayout.ABC', 'actions': acts, **(extra or {})}
    (d / (name + '.json')).write_text(json.dumps(job))
    return subprocess.Popen([str(BIN / 'gui-probe'), '--job', str(d / (name + '.json')), '--out', str(d / name)],
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)


def wait_ready(cfg):
    # MyGo applies the carets in its first laid-out frame after /setup: the
    # state before typing is the one /state reports ready.
    for _ in range(60):
        st = call(cfg, '/state')
        if st['ready']:
            return st
        time.sleep(.05)
    raise RuntimeError('editor not ready after setup')


def setup(cfg, doc, carets):
    call(cfg, '/setup', {'scenario': doc, 'carets': carets})
    return wait_ready(cfg)


def refocus_and_setup(cfg, doc, carets, before):
    # Tango needs a click for focus and the click collapses the carets: wait
    # for it to land, then re-apply the document and carets.
    deadline = time.time() + 6
    while time.time() < deadline:
        st = call(cfg, '/state')
        if st['focused'] and st['selections'] != before['selections']:
            return setup(cfg, doc, carets)
        time.sleep(.03)
    raise RuntimeError('click did not land in time')


def run_probe(d, name, cfg, w, acts, click, doc, carets, before, extra=None):
    pp = probe_bg(d, name, cfg, w, ([click] if click else []) + acts, extra)
    if click:
        refocus_and_setup(cfg, doc, carets, before)
    _, err = pp.communicate(timeout=900)
    return pp.returncode, err[-200:]


def run(name):
    d = OUT / name
    shutil.rmtree(d, ignore_errors=True)
    d.mkdir()
    ready = d / 'ready.json'
    proc = subprocess.Popen([str(BIN / FIXTURE), '-ready', str(ready)], stdout=open(d / 'fixture.log', 'w'), stderr=subprocess.STDOUT)
    res = {'scenario': name, 'engine': opt.engine}
    try:
        for _ in range(100):
            if ready.exists():
                break
            if proc.poll() is not None:
                raise RuntimeError('fixture exited')
            time.sleep(.1)
        cfg = json.loads(ready.read_text())
        doc, carets = ('multi', 3) if name == 'multi' else ('type', 1)
        keys = opt.multi_keys if name == 'multi' else opt.keys
        st = setup(cfg, doc, carets)
        res['renderer'] = st.get('renderer') or cfg.get('renderer')
        w = next(x for x in json.loads(subprocess.check_output([str(BIN / 'gui-probe'), '--list'])) if x['pid'] == cfg['pid'])
        fx, fy, fw, fh = w['frame']
        bx, by, bw, bh = st['body']
        click = {'id': 100, 'kind': 'click', 'x': fx + 150, 'y': fy + 60, 'waitMS': 1500} if NEEDS_CLICK and name != 'scroll' else None
        if name == 'scroll':
            # Inset from the window edge: an aligned patch may move up to 4 px.
            regions = [{'x': 16, 'y': int((by + bh * 0.6) * SCALE), 'width': 800, 'height': 120}]
            inputs = lambda extra: [dict({'id': i + 1, 'kind': 'scroll', 'dy': -40, 'x': fx + fw / 2, 'y': fy + fh / 2}, **extra) for i in range(opt.scrolls)]
            rec_wait, settle = 700, 250
        else:
            # The body's width (the window frame can be a point wider than the
            # capture): 100 typed columns run past 800 points.
            regions = [{'x': 0, 'y': math.floor(t * SCALE), 'width': int(bw * SCALE), 'height': math.ceil(h * SCALE)} for t, h in st['rows']]
            inputs = lambda extra: [dict({'id': i + 1, 'kind': 'key', 'keycode': 7}, **extra) for i in range(keys)]
            rec_wait, settle = 1300, 300
        res['regions'] = regions
        before = st
        if opt.templates_from:
            src = pathlib.Path(opt.templates_from).resolve() / name
            if not (src / 'templates' / 'patches.json').exists():
                raise RuntimeError('the reused record pass is missing: ' + str(src))
            saved = json.loads((src / 'regions.json').read_text())
            # A relaunched editor may lay its rows out a pixel off: keep the
            # recorded regions when they match closely, alignment does the rest.
            if len(saved) != len(regions) or any(a['width'] != b['width'] or a['height'] != b['height'] or
                                                 abs(a['x'] - b['x']) > 4 or abs(a['y'] - b['y']) > 4 for a, b in zip(saved, regions)):
                raise RuntimeError(f'regions differ from the reused record pass: {saved} vs {regions}')
            regions = saved
            res['regions'] = regions
            tdir, rdir = src / 'templates', src / 'record'
            res['templatesFrom'] = str(src)
        else:
            rec = [{'id': 0, 'kind': 'wait', 'waitMS': rec_wait, 'settleMS': settle, 'record': regions}] + \
                  inputs({'waitMS': rec_wait, 'settleMS': settle, 'record': regions})
            code, err = run_probe(d, 'record', cfg, w, rec, click, doc, carets, before)
            res['recordProbe'] = {'exit': code, 'err': err}
            if code:
                raise RuntimeError('record probe failed: ' + err)
            (d / 'regions.json').write_text(json.dumps(regions))
            cols = ','.join(str(x['head'] - (before['text'].rfind('\n', 0, x['head']) + 1)) for x in before['selections'])
            rc, line, e = node(HERE / 'make-templates.mjs', '--capture', d / 'record', '--kind', 'scroll' if name == 'scroll' else 'key',
                               '--columns', cols, '--out', d / 'templates')
            res['templates'] = json.loads(line) if line.startswith('{') else {'ok': False, 'err': e}
            if rc:
                raise RuntimeError('templates unusable')
            tdir, rdir = d / 'templates', d / 'record'
            before = setup(cfg, doc, carets)
        patches = {p['id']: p['patches'] for p in json.loads((tdir / 'patches.json').read_text())}
        # The lead-in equals the record pass's baseline wait (an app may drop
        # the first wheel event right after activation) and records the
        # regions, to align to the record baseline; a failed alignment fails
        # the pass. The trailing wait records the end state for diagnosis.
        meas = [{'id': 0, 'kind': 'wait', 'waitMS': rec_wait, 'settleMS': settle, 'record': regions}] + \
               [dict(a, patches=patches[a['id']]) for a in inputs({'waitMS': 1000})] + \
               [{'id': 999, 'kind': 'wait', 'waitMS': rec_wait, 'settleMS': settle, 'record': regions}]
        align = {'alignTo': [str(rdir / f'rec-0-{i}.bgra') for i in range(len(regions))], 'alignAfter': 0}
        code, err = run_probe(d, 'measure', cfg, w, meas, click, doc, carets, before, align)
        res['measureProbe'] = {'exit': code, 'err': err}
        try:
            res['align'] = next(json.loads(l) for l in open(d / 'measure' / 'capture.ndjson') if '"align"' in l)
        except (StopIteration, OSError):
            res['align'] = None
        rc, line, e = node(HERE / 'latency.mjs', d / 'measure' / 'capture.ndjson')
        res['latency'] = json.loads(line) if line.startswith('{') else {'err': e}
        after = call(cfg, '/state')
        offs = [x['head'] for x in before['selections']]
        b = before['text'].encode()
        if name != 'scroll':
            for o in sorted(offs, reverse=True):
                b = b[:o] + b'x' * keys + b[o:]
        res['textOK'] = after['text'] == b.decode()
        call(cfg, '/quit')
    except Exception as ex:
        res['error'] = str(ex)
    finally:
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
    return res


results = [run(n) for n in opt.scenarios.split(',')]
(OUT / 'summary.json').write_text(json.dumps(results, indent=2))
print(json.dumps(results))
sys.exit(0 if all(r.get('textOK') and (r.get('latency') or {}).get('complete') == (r.get('latency') or {}).get('posted') for r in results) else 1)
