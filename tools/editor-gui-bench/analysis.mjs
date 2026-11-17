// Pure analysis of one observer's host-clock records. All times use the same
// unit (the native probe emits mach host nanoseconds). No callback wall time,
// interpolation or omission of failed input is used to make a result faster.
export function latencyBounds(event, frames) {
  const { id, postedBefore: before, postedAfter: after, deadline } = event;
  const censored = { id, lower: Math.max(0, deadline - after), upper: null, complete: false, valid: true, first: null, all: null };
  if (![before, after, deadline].every(Number.isFinite) || after < before || deadline < after) return { ...censored, valid: false };
  let last = -Infinity, unchanged = before, first = null;
  for (const f of frames) {
    if (!Number.isFinite(f.time) || f.time < last || f.dropped || f.scale !== event.scale || f.width !== event.width || f.height !== event.height) return { ...censored, valid: false };
    last = f.time;
    if (!['complete', 'idle', 'started'].includes(f.status)) return { ...censored, valid: false };
    // Idle is evidence of unchanged display, but cannot carry a new match.
    if (f.time < after || f.status !== 'complete') continue;
    if (f.time > deadline) break;
    if (f.matches > 0 && first === null) first = f.time;
    if (f.matches >= event.regions) return { id, lower: Math.max(0, unchanged - after), upper: f.time - before, complete: true, valid: true, first, all: f.time };
    unchanged = f.time;
  }
  return { ...censored, first };
}

const percentile = (a, p) => a.length ? a.toSorted((x, y) => x - y)[Math.ceil(p * a.length) - 1] : Infinity;
export function summarize(results) {
  const lower = percentile(results.map(r => r.lower), .95);
  const upper = percentile(results.map(r => r.upper ?? Infinity), .95);
  const invalid = results.filter(r => !r.valid).length;
  return { posted: results.length, complete: results.filter(r => r.complete).length, invalid,
    censored: results.filter(r => !r.complete).length, p95Lower: Number.isFinite(lower) ? lower : null,
    p95Upper: Number.isFinite(upper) ? upper : null,
    verdict: invalid || !Number.isFinite(upper) ? 'not-proven' : 'observed-only' };
}

export function comparePairs(a, b) {
  return a.map(x => {
    const y = b.find(y => y.pair === x.pair);
    const conclusion = !y || x.p95Upper == null || y.p95Upper == null ? 'not-proven' :
      x.p95Upper < y.p95Lower ? 'a-observed-faster' : y.p95Upper < x.p95Lower ? 'b-observed-faster' : 'cannot-distinguish';
    return { pair: x.pair, conclusion, a: [x.p95Lower, x.p95Upper], b: [y?.p95Lower ?? null, y?.p95Upper ?? null] };
  });
}
