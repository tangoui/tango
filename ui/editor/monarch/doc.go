// Package monarch interprets Monaco Monarch grammars (JSON, as exported by
// ui/editor/langs) and tokenizes text line by line.
//
// A Registry maps language ids to grammars and compiles a Lexer on first use;
// Lexer.TokenizeLine takes the previous line's end State and returns the
// line's tokens and its end State. Semantics follow Monaco's monarchLexer,
// checked against Monaco's own tokenization tests.
//
// Contracts:
//
//   - Degraded lines. Grammars from plugins are untrusted. Pathological
//     backtracking cannot stall a line: a single regexp2 match times out after
//     regex.MatchTimeout, and a line degrades on the first timeout or once its
//     tokenization exceeds LineBudget. A degraded line keeps the tokens produced
//     before that point, covers the rest with one "" token and returns its
//     input State, exactly like the long-line rule (MaxLineLength).
//     TokenizeLineInfo reports this as LineInfo.Degraded (TokenizeLine drops
//     it); Stats counts degraded lines. Because the returned State may differ
//     from the true end state, a caller must not treat "end state unchanged"
//     on a degraded line as a reason to stop re-tokenizing the lines below.
//
//   - Runtime errors. A rule that would make Monaco's tokenizer throw, a
//     no-progress loop, or a recovered panic yields one "" token for the whole
//     line and the input State (counted in Stats).
//
//   - Registry.Add. After Add(id, ...) replaces a grammar, every State
//     previously produced for that language is invalid for the new Lexer —
//     including States of other languages that embed it (they hold the old
//     embedded Lexer). Callers must re-tokenize affected documents from
//     Lexer.Initial(). The old Lexer itself keeps working on its own States.
//
//   - Compilation. A grammar that cannot compile (undefined states or
//     attributes, @attr expansion beyond 64 KiB, a panic in the
//     compiler) makes Registry.Lexer return an error; the result is cached.
//     Compilation happens outside the Registry lock.
package monarch
