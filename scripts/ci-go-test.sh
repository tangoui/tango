#!/usr/bin/env bash
# Runs `go test "$@"` and, inside GitHub Actions, reports the failures as
# error annotations. Annotations can be read without signing in (job logs
# cannot), so a failed CI run shows which tests failed and why.
#
#   bash scripts/ci-go-test.sh -race -timeout 20m ./...
set -uo pipefail

log="$(mktemp)"
go test "$@" 2>&1 | tee "$log"
status=${PIPESTATUS[0]}

if ((status != 0)) && [[ -n "${GITHUB_ACTIONS:-}" ]]; then
	# Failed tests and packages, panics and timeouts, and the test's own
	# messages (file_test.go:NN: ...), at most 30 lines.
	grep -E '^(--- FAIL|FAIL|panic:)|_test\.go:[0-9]+:|^\s+Error' "$log" | head -n 30 |
		while IFS= read -r line; do
			line="${line//%/%25}"
			echo "::error title=go test::${line}"
		done
fi
rm -f "$log"
exit "$status"
