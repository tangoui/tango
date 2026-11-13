#!/usr/bin/env bash
# Checks the public Go API of github.com/tangoui/tango for incompatible
# changes against a base commit, using golang.org/x/exp/cmd/apidiff.
# Used both locally and by CI (.github/workflows/api.yml); see docs/ci.md.
#
#   bash scripts/apidiff.sh               compare the working tree with origin/main
#   bash scripts/apidiff.sh BASE          compare with BASE (commit, tag or branch)
#
# The working tree (including uncommitted changes) is the "new" side; BASE
# is checked out into a temporary git worktree. The API is exported for
# linux, darwin and windows (CGO_ENABLED=0) so platform-specific files are
# covered. Internal packages are ignored by apidiff.
#
# Packages are split into two tiers (docs/release.md, "API 稳定性"):
#   experimental  ui, uihost, mobile, service/updater, examples: incompatible
#                 changes are reported as warnings only
#   stable        everything else: an incompatible change fails the check
#
# Exit status: 0 no incompatible change in stable packages (or allowed),
# 1 incompatible change in stable packages, 2 setup error.
#
# Environment (all optional):
#   ALLOW_API_BREAK=1   report stable breaks but exit 0 (CI sets it for PRs
#                       labelled api-break or pushes whose message has [api-break])
#   APIDIFF             apidiff binary to use (default: install the pinned version)
#   APIDIFF_GOOS        platforms to check (default "linux darwin windows")
set -euo pipefail

MODULE=github.com/tangoui/tango
APIDIFF_VERSION=v0.0.0-20261112173441-d10d3b464990   # golang.org/x/exp
EXPERIMENTAL_RE='^- \./(ui|uihost|mobile|service/updater|examples)([./:]|$)'

root="$(git rev-parse --show-toplevel)"
cd "$root"

base_ref="${1:-origin/main}"
if ! base_sha="$(git rev-parse --verify --quiet "${base_ref}^{commit}")"; then
  echo "apidiff: unknown base '$base_ref'" >&2
  exit 2
fi

tmp="$(mktemp -d)"
cleanup() {
  git -C "$root" worktree remove --force "$tmp/base" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

apidiff="${APIDIFF:-}"
if [ -z "$apidiff" ]; then
  # Build for the host: GOOS below only applies to loading the module.
  GOBIN="$tmp/bin" GOOS= GOARCH= GOWORK=off go install "golang.org/x/exp/cmd/apidiff@$APIDIFF_VERSION"
  apidiff="$tmp/bin/apidiff"
fi

git worktree add --quiet --detach "$tmp/base" "$base_sha"

# export DIR OUT GOOS: write the module's export data, dropping apidiff's
# "Ignoring internal package" chatter but keeping real errors.
export_api() {
  local dir="$1" out="$2" goos="$3" log="$2.log"
  if ! (cd "$dir" && GOWORK=off GOOS="$goos" GOARCH=amd64 CGO_ENABLED=0 \
        "$apidiff" -m -w "$out" "$MODULE") >"$log" 2>&1; then
    grep -v '^Ignoring internal package' "$log" >&2 || true
    echo "apidiff: exporting the API of $dir for $goos failed" >&2
    exit 2
  fi
}

goos_list="${APIDIFF_GOOS:-linux darwin windows}"
: >"$tmp/all.txt"
for goos in $goos_list; do
  export_api "$tmp/base" "$tmp/old-$goos" "$goos"
  export_api "$root" "$tmp/new-$goos" "$goos"
  "$apidiff" -m -incompatible "$tmp/old-$goos" "$tmp/new-$goos" 2>/dev/null \
    | grep '^- ' | sed "s/\$/  [$goos]/" >>"$tmp/all.txt" || true
done

# Merge the per-platform lists: one line per change, platforms joined.
merged="$(awk '
  { plat = $NF; sub(/  \[[a-z]+\]$/, ""); if (!($0 in p)) order[++n] = $0; p[$0] = p[$0] (p[$0] ? "," : "") substr(plat, 2, length(plat) - 2) }
  END { for (i = 1; i <= n; i++) { k = order[i]; print k "  [" p[k] "]" } }
' "$tmp/all.txt")"

stable="$(printf '%s\n' "$merged" | grep -Ev "$EXPERIMENTAL_RE" | grep . || true)"
experimental="$(printf '%s\n' "$merged" | grep -E "$EXPERIMENTAL_RE" || true)"

short="$(git rev-parse --short "$base_sha")"
report="$tmp/report.md"
{
  echo "## Go API 兼容性（对比 ${short}）"
  echo
  if [ -n "$stable" ]; then
    echo "### 稳定包的不兼容变化"
    echo '```'
    printf '%s\n' "$stable"
    echo '```'
  else
    echo "稳定包没有不兼容变化。"
  fi
  if [ -n "$experimental" ]; then
    echo
    echo "### 实验性包的不兼容变化（仅提示）"
    echo '```'
    printf '%s\n' "$experimental"
    echo '```'
  fi
} >"$report"

cat "$report"
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  cat "$report" >>"$GITHUB_STEP_SUMMARY"
fi

if [ -n "$experimental" ] && [ -n "${GITHUB_ACTIONS:-}" ]; then
  echo "::warning::experimental packages have incompatible API changes (see job summary)"
fi

if [ -n "$stable" ]; then
  if [ "${ALLOW_API_BREAK:-}" = 1 ]; then
    [ -n "${GITHUB_ACTIONS:-}" ] && echo "::warning::incompatible API changes in stable packages (allowed by api-break)"
    echo
    echo "ALLOW_API_BREAK=1: allowed. Describe the change and the migration in CHANGELOG.md."
    exit 0
  fi
  [ -n "${GITHUB_ACTIONS:-}" ] && echo "::error::incompatible API changes in stable packages (see job summary)"
  echo
  echo "Incompatible changes in stable packages. Keep the old API (add new"
  echo "functions/options instead), or if the break is intended: label the PR"
  echo "api-break (or put [api-break] in the commit message) and describe the"
  echo "migration in CHANGELOG.md."
  exit 1
fi
