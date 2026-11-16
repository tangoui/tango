# Monarch Fast Execution Design

## Intent

Reduce built-in HTML/JavaScript/CSS tokenization cost while preserving Monarch grammar behavior for custom and complex grammars.

## Architecture

Compile conservative metadata beside each Monarch rule: an ASCII candidate set and, where semantics permit, a consuming RE2 pattern plus a post-match assertion. The tokenizer uses indexed candidates and direct boundary scanners only for proven cases; all unsupported constructs continue through the existing RE2/regexp2 path.

The HTML fast path is state-aware and handles only fixed embedded boundaries and ordinary byte scanning. It must preserve token offsets, line state, captures, `@rematch`, `goBack`, budgets, timeout behavior, and concurrent Lexer use.

## Constraints

- No grammar behavior changes for custom registries or unsupported patterns.
- No dependency fork in the first phase.
- Every fast path requires differential tests against the existing tokenizer.
- MyGo comparison remains diagnostic; no claim of superiority without five low-load paired samples.
