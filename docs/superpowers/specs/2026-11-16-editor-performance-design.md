# Editor allocation performance optimization

The user requested editor performance improvements. Preserve the current workbench, editing semantics and enabled display features. Prior instructions delegate routine choices to the implementer.

Measured baseline on Apple M4/darwin/arm64 (1920×1080 logical, 2× offscreen Metal, full redraw, 20k TypeScript lines, 100 iterations, three runs): typing median ~6.05ms / 1.11MB / 9921 allocations per operation; scrolling median ~4.94ms / 949KB / 5110 allocations. These are benchmark frames ending at GPU completion, not physical input latency or display FPS. Profile includes setup/highlighting and points to temporary token copies and style generation as substantial allocation sources.

Use retained per-view scratch for minimap token queries and keep-set bookkeeping, bounded stack storage for common minimap glyph cells, and reusable per-view shaping input runs/override-boundary scratch. Persistent minimap tokens and painted run colors remain independent; LineCache copies shaping keys on cache miss. Do not disable any feature, change public APIs, alter grammar semantics or introduce unbounded caches.

Alternatives considered: disable minimap/highlight (rejected: functionality tradeoff), change lexer/renderer architecture (deferred: larger risk than supported by this profile), reuse existing temporary buffers (chosen: small and independently verifiable).

Validate allocation regressions, existing minimap pixel/token/grammar invalidation tests, editor damage/golden tests, race tests on changed packages and full suite. Repeat the exact baseline benchmarks without mixing physical display and offscreen timings; report allocation changes even when GPU-bound timing is noisy.
