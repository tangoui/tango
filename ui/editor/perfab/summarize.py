"""Summarizes an arms.sh (or run.sh) log directory as Markdown.

    python3 -I summarize.py OUT REF_LABEL [LABEL ...]

Reads the rounds kept (OUT/rounds.txt), the benchmark result lines of
every arm's logs (OUT/<label>/<round>.log) and the load samples
(OUT/load.log). For every arm other than the reference REF_LABEL it prints,
per benchmark and metric, the median over the rounds of the reference and
of the arm, the arm/reference ratio, the "worse" ratio (the ratio for
times, its inverse for fps: above 1 means the arm is slower) with a 95%
bootstrap interval (rounds resampled in pairs, the rounds being paired:
the arms of a round run back to back), and a verdict.

With $AA set to the label of an A/A arm (the reference run a second time,
interleaved in the same rounds), the A/A "worse" ratio of each metric is
the noise of that metric: spread = |A/A worse - 1|, the A/A arm compared
with $AA_REF (default REF_LABEL: the arm it copies, when the reference of
the summary is another tree, e.g. milestone 1). The verdict of an arm's
metric is then
    regression    worse > 1.10 and worse - 1 > spread
    within noise  worse > 1.10 but worse - 1 <= spread
and empty otherwise. The 1.10 gate is the plan's 10% (Global Constraints).

With COST=1 the arms are baselines with something turned off (OFF=<feature>
against the default reference): the "worse" column, its interval and the
verdict are then the reference's cost over the arm (the inverse: above 1
means the feature makes the reference slower). The A/A arm is never
inverted.
Without $AA the spread is 0. OUT/summary.json holds the same numbers.
"""

import json
import os
import random
import re
import statistics
import sys

GATE = 1.10
out = sys.argv[1]
labels = sys.argv[2:]
ref = labels[0]
aa = os.environ.get("AA", "")
aa_ref = os.environ.get("AA_REF", "") or ref
cost = os.environ.get("COST", "") == "1"

# Metrics in the summary, in order (others are ignored); GATED are the
# plan's three gated quantities (input p95, scroll fps and p99, open).
METRICS = ["open-ms", "hl-ms", "p50-ms", "p95-ms", "p99-ms", "max-ms",
           "fps", "mean-ms", "cpu-ms", "gpuwait-ms", "partial"]
GATED = {("TypeAt1MLines", "p95-ms"), ("Scroll", "fps"), ("Scroll", "p99-ms"),
         ("Open100MB", "open-ms")}
HIGHER_BETTER = {"fps", "partial"}

line_re = re.compile(r"^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+(.*)$")


def parse(path):
    res = {}
    with open(path) as f:
        for line in f:
            m = line_re.match(line.rstrip("\n"))
            if not m:
                continue
            rest = m.group(2).split()
            vals = {}
            for i in range(0, len(rest) - 1, 2):
                try:
                    vals[rest[i + 1]] = float(rest[i])
                except ValueError:
                    pass
            res[m.group(1).removeprefix("Benchmark")] = vals
    return res


def gated(bench, met):
    return (bench.split("/")[0], met) in GATED


with open(os.path.join(out, "rounds.txt")) as f:
    rounds = [r.strip() for r in f if r.strip()]
data = {l: [parse(os.path.join(out, l, f"{r}.log")) for r in rounds] for l in labels}
# A discarded round is run again under the same number: the last samples
# of a round and arm are those of the kept run.
last = {}
if os.path.exists(os.path.join(out, "load.log")):
    with open(os.path.join(out, "load.log")) as f:
        for line in f:
            # "round label load0 load1", or with arms.sh's screen state
            # "round label load0 load1 screen0 screen1".
            p = line.split()
            if len(p) in (4, 6) and p[0] in rounds and p[1] in labels:
                screen = (p[4], p[5]) if len(p) == 6 else None
                last[(p[0], p[1])] = (p[0], p[1], float(p[2]), float(p[3]), screen)
loads = list(last.values())

benches = []
for l in labels:
    for d in data[l]:
        for k in d:
            if k not in benches:
                benches.append(k)


def paired(label, bench, met, base):
    """The (base, arm) values of the rounds where both have met."""
    pr = []
    for dr, da in zip(data[base], data[label]):
        if met in dr.get(bench, {}) and met in da.get(bench, {}):
            pr.append((dr[bench][met], da[bench][met]))
    return pr


def worse(met, mr, ma):
    if met in HIGHER_BETTER:
        return mr / ma if ma else float("nan")
    return ma / mr if mr else float("nan")


def stats(label, bench, met, base=ref):
    pr = paired(label, bench, met, base)
    if not pr:
        return None
    rv, av = [p[0] for p in pr], [p[1] for p in pr]
    mr, ma = statistics.median(rv), statistics.median(av)
    rng = random.Random(1)
    boots = []
    for _ in range(2000):
        s = [pr[rng.randrange(len(pr))] for _ in pr]
        boots.append(worse(met, statistics.median(p[0] for p in s), statistics.median(p[1] for p in s)))
    boots.sort()
    return {"ref": mr, "arm": ma, "ratio": ma / mr if mr else float("nan"),
            "worse": worse(met, mr, ma), "lo": boots[int(0.025 * len(boots))],
            "hi": boots[int(0.975 * len(boots)) - 1], "n": len(pr), "refv": rv, "armv": av}


def fmt(v):
    return f"{v:.3f}" if abs(v) < 100 else f"{v:.1f}"


noise = {}
if aa and aa in labels:
    for bench in benches:
        for met in METRICS:
            s = stats(aa, bench, met, aa_ref)
            if s:
                noise[(bench, met)] = abs(s["worse"] - 1)

print(f"Reference {ref}; {len(rounds)} rounds, arms interleaved (order rotated per round)"
      + (f"; A/A arm {aa} (copy of {aa_ref})" if aa else "") + "\n")
summary = {"ref": ref, "aa": aa, "aa_ref": aa_ref, "cost": cost, "rounds": len(rounds), "arms": {}}
for label in labels[1:]:
    base = aa_ref if label == aa else ref
    print(f"## {label} vs {base}\n")
    wc = "cost (ref over arm)" if cost and label != aa else "worse"
    print(f"| benchmark | metric | gated | ref median | arm median | arm/ref | {wc} | {wc} 95% CI | A/A spread | verdict | rounds ref | rounds arm |")
    print("|---|---|---|---|---|---|---|---|---|---|---|---|")
    rows = []
    for bench in benches:
        for met in METRICS:
            s = stats(label, bench, met, base)
            if not s:
                continue
            if cost and label != aa:
                s["worse"], s["lo"], s["hi"] = 1 / s["worse"], 1 / s["hi"], 1 / s["lo"]
            sp = noise.get((bench, met), 0.0)
            verdict = ""
            if label != aa and s["worse"] > GATE:
                verdict = "regression" if s["worse"] - 1 > sp else "within noise"
            s.update(bench=bench, metric=met, gated=gated(bench, met), spread=sp, verdict=verdict)
            rows.append(s)
            print(f"| {bench} | {met} | {'yes' if s['gated'] else ''} | {fmt(s['ref'])} | {fmt(s['arm'])} | "
                  f"{s['ratio']:.3f} | {s['worse']:.3f} | {s['lo']:.3f}-{s['hi']:.3f} | "
                  f"{sp * 100:.1f}% | {verdict} | {' '.join(fmt(v) for v in s['refv'])} | "
                  f"{' '.join(fmt(v) for v in s['armv'])} |")
    print()
    summary["arms"][label] = rows

if loads:
    allv = [v for l in loads for v in l[2:4]]
    print(f"Load (1-minute uptime) over the kept rounds: {min(allv):.2f}-{max(allv):.2f}\n")

    def arm_load(l):
        s = f"{l[1]} {l[2]:.2f}->{l[3]:.2f}"
        return s + (f" ({l[4][0]}->{l[4][1]})" if l[4] else "")

    for r in rounds:
        print(f"- round {r}: " + ", ".join(arm_load(l) for l in loads if l[0] == r))
    summary["load"] = [min(allv), max(allv)]
    screens = sorted({s for l in loads if l[4] for s in l[4]})
    if screens:
        print(f"\nScreen state over the kept rounds: {', '.join(screens)}")
        # Locked or changing screens make offscreen GPU timings unlike an
        # unlocked session's: report them, never mix them in silently.
        odd = sorted({l[0] for l in loads if l[4] and (l[4][0] != "unlocked" or l[4][1] != "unlocked")}, key=int)
        if odd:
            print(f"WARNING: rounds not unlocked throughout: {' '.join(odd)}; group them separately")
        summary["screen"] = {"states": screens, "not_unlocked_rounds": odd}
with open(os.path.join(out, "summary.json"), "w") as f:
    json.dump(summary, f, indent=1)
