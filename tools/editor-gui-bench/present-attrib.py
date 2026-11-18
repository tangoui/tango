#!/usr/bin/env python3
"""Present attribution for one native-driver key scenario run with
TANGO_PRESENT_TRACE=1 (Tango on Metal).

  present-attrib.py <scenario dir>     e.g. out/type

Per measured key k, all on one clock (uptime ns):
  post      probe postedAfter of key k
  received  the fixture observer's k-th TextInput
  submit    submittedNS of the drawable that produced the matching frame
  presented presentedNS (Core Animation's presentedTime) of that drawable
  matched   ScreenCaptureKit displayTime of the first all-regions match
The drawable is the first one submitted after the key was received. Core
Animation's presentedTime can be a few ms after ScreenCaptureKit's
displayTime of the same picture, so presented→matched may be negative.
Prints per-stage medians and p90 in ms.
"""
import json, math, pathlib, statistics as st, sys

d = pathlib.Path(sys.argv[1])
rows = [json.loads(l) for l in open(d / 'measure' / 'capture.ndjson')]
inputs = {r['id']: r for r in rows if r['type'] == 'input' and r.get('regions', 0) > 0}
match = {}
for r in rows:
    if r['type'] == 'frame' and r.get('status') == 'complete' and r.get('id') in inputs and r['id'] not in match:
        if r.get('matches', 0) >= inputs[r['id']]['regions'] and r.get('time'):
            match[r['id']] = r['time']
texts = [r['Host'] for r in json.loads((d / 'input-trace.json').read_text()) if r['Kind'] == 'text']
presents = []
for line in open(d / 'fixture.log'):
    line = line.strip()
    if line.startswith('{') and '"metal-present"' in line:
        r = json.loads(line)
        if r.get('status') == 'presented' and r.get('presentedNS', 0) > 0:
            presents.append(r)
presents.sort(key=lambda r: r['presentedNS'])
ids = sorted(inputs)
if len(texts) != len(ids):
    sys.exit(f'{len(texts)} observed text inputs for {len(ids)} keys: cannot pair them')
stages = {'post→received': [], 'received→submit': [], 'submit→presented': [], 'presented→matched': [], 'post→matched': []}
early = missing = 0
for k, recv in zip(ids, texts):
    post, m = inputs[k]['postedAfter'], match.get(k)
    if m is None:
        missing += 1
        continue
    after = [p for p in presents if p['submittedNS'] >= recv]
    if not after:
        missing += 1
        continue
    p = min(after, key=lambda p: p['submittedNS'])
    if p['presentedNS'] > m + 20e6:
        early += 1  # the picture matched before this drawable could show: not attributable
        continue
    for name, v in (('post→received', recv - post), ('received→submit', p['submittedNS'] - recv),
                    ('submit→presented', p['presentedNS'] - p['submittedNS']), ('presented→matched', m - p['presentedNS']),
                    ('post→matched', m - post)):
        stages[name].append(v / 1e6)
print(f'{d}: {len(ids)} keys, {len(presents)} presented drawables, attributed {len(stages["post→matched"])}, '
      f'drawable after the match {early}, no match/drawable {missing}')
for name, v in stages.items():
    if v:
        v.sort()
        print(f'  {name:18} median {st.median(v):6.1f} ms   p90 {v[math.ceil(.9 * len(v)) - 1]:6.1f} ms')
