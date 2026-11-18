#!/usr/bin/env python3
"""Formal GUI matrix: three editors in a Latin-square order over three rounds.

  matrix.py --bin <dir> --out <dir> [--keys 100] [--multi-keys 30] [--scrolls 240]
            [--engines tango,mygo,monaco] [--max-load 6]

Round 1 records templates per engine; rounds 2 and 3 reuse them (the
measure pass aligns to the round-1 baseline). Each run waits for the
1-minute load to drop below --max-load (up to 5 minutes, recorded either
way). Results: <out>/r<k>/<engine>/summary.json and <out>/matrix.json.
"""
import argparse, json, pathlib, subprocess, sys, time

HERE = pathlib.Path(__file__).resolve().parent
ap = argparse.ArgumentParser()
ap.add_argument('--bin', required=True)
ap.add_argument('--out', required=True)
ap.add_argument('--keys', type=int, default=100)
ap.add_argument('--multi-keys', type=int, default=30)
ap.add_argument('--scrolls', type=int, default=240)
ap.add_argument('--engines', default='tango,mygo,monaco')
ap.add_argument('--max-load', type=float, default=6)
opt = ap.parse_args()
BIN, OUT = pathlib.Path(opt.bin).resolve(), pathlib.Path(opt.out).resolve()
E = opt.engines.split(',')
ORDERS = [E[i:] + E[:i] for i in range(len(E))]  # Latin square: each engine first once


def load1():
    return float(subprocess.check_output(['sysctl', '-n', 'vm.loadavg'], text=True).split()[1])


def run(rnd, engine):
    out = OUT / f'r{rnd}' / engine
    out.mkdir(parents=True, exist_ok=True)
    deadline = time.time() + 300
    while load1() >= opt.max_load and time.time() < deadline:
        time.sleep(10)
    meta = {'round': rnd, 'engine': engine, 'start': time.strftime('%F %T'), 'loadStart': load1()}
    reuse = [] if rnd == 1 else ['--templates-from', str(OUT / 'r1' / engine)]
    if engine == 'monaco':
        cmd = ['node', str(HERE / 'monaco-driver.mjs'), '--templates', '--probe', str(BIN / 'gui-probe'), '--out', str(out),
               '--keys', str(opt.keys), '--multi-keys', str(opt.multi_keys), '--scrolls', str(opt.scrolls), *reuse]
    else:
        cmd = [sys.executable, str(HERE / 'native-driver.py'), '--engine', engine, '--bin', str(BIN), '--out', str(out),
               '--keys', str(opt.keys), '--multi-keys', str(opt.multi_keys), '--scrolls', str(opt.scrolls), *reuse]
    p = subprocess.run(cmd, capture_output=True, text=True)
    (out / 'driver.log').write_text(p.stdout + p.stderr)
    meta.update({'end': time.strftime('%F %T'), 'loadEnd': load1(), 'exit': p.returncode})
    try:
        meta['results'] = json.loads((out / 'summary.json').read_text())
    except (OSError, ValueError):
        meta['results'] = None
    return meta


runs = []
for rnd, order in enumerate(ORDERS, 1):
    for engine in order:
        m = run(rnd, engine)
        runs.append(m)
        (OUT / 'matrix.json').write_text(json.dumps(runs, indent=2))
        for r in m['results'] or []:
            lat = r.get('latency') or {}
            ok = r.get('textOK', r.get('pass'))
            print(f"r{rnd} {engine:6} {r['scenario']:6} text={ok} {lat.get('complete')}/{lat.get('posted')} "
                  f"p95=[{lat.get('p95Lower')}, {lat.get('p95Upper')}] align={(r.get('align') or {}).get('dx')} err={(r.get('error') or '')[:60]}", flush=True)
print('matrix done', time.strftime('%F %T'))
