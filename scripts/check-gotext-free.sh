#!/usr/bin/env bash
# Fails if the default macOS build of the self-drawn UI links go-text.
set -euo pipefail
deps=$(GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go list -deps ./ui/... ./uihost/...)
if grep -q '^github.com/go-text/' <<<"$deps"; then
  echo "go-text is linked into the default darwin build:" >&2
  grep '^github.com/go-text/' <<<"$deps" >&2
  exit 1
fi
