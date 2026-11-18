#!/usr/bin/env bash
# Fails if any package of the module links go-text in the default macOS build.
set -euo pipefail
deps=$(GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go list -deps ./...)
if grep -q '^github.com/go-text/' <<<"$deps"; then
  echo "go-text is linked into the default darwin build:" >&2
  grep '^github.com/go-text/' <<<"$deps" >&2
  exit 1
fi
