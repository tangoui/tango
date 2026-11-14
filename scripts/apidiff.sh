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
# Tiers come from api/tiers.txt and the allowlists api/stable/<pkg>.txt of
# the working tree (docs/release.md, "API 稳定性"); internal/cmd/apitxt
# -classify applies them to each reported change:
#   stable        a change in a stable package, or of an allowlisted symbol
#                 of a split package: fails the check
#   experimental  everything else (experimental and excluded packages,
#                 non-allowlisted symbols of split packages): warning only
# A package missing from api/tiers.txt (for example one that was removed)
# counts as stable.
#
# Exit status: 0 no incompatible stable change (or allowed), 1 incompatible
# stable change, 2 setup error.
#
# Environment (all optional):
#   ALLOW_API_BREAK=1   report stable breaks but exit 0 (CI sets it for PRs
#                       labelled api-break or pushes whose message has [api-break])
#   APIDIFF             apidiff binary to use (default: install the pinned version)
#   APIDIFF_GOOS        platforms to check (default "linux darwin windows")
set -euo pipefail

MODULE=github.com/tangoui/tango
APIDIFF_VERSION=v0.0.0-20261112173441-d10d3b464990   # golang.org/x/exp

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

# The tier classifier is built from the working tree.
if ! GOOS= GOARCH= GOWORK=off go build -o "$tmp/bin/apitxt" ./internal/cmd/apitxt; then
  echo "apidiff: building internal/cmd/apitxt failed" >&2
  exit 2
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

# Prefix every change with its tier ("stable<TAB>- ./ui.Button: ...").
if ! classified="$(printf '%s\n' "$merged" | "$tmp/bin/apitxt" -classify -C "$root")"; then
  echo "apidiff: classifying the changes failed (api/tiers.txt, api/stable)" >&2
  exit 2
fi
pick() {
  printf '%s\n' "$classified" | awk -v tier="$1" '
    { i = index($0, "\t"); if (i > 0 && substr($0, 1, i - 1) == tier) print substr($0, i + 1) }'
}
stable="$(pick stable)"
experimental="$(pick experimental)"

short="$(git rev-parse --short "$base_sha")"
report="$tmp/report.md"
{
  echo "## Go API 兼容性（对比 ${short}）"
  echo
  if [ -n "$stable" ]; then
    echo "### 稳定层的不兼容变化"
    echo '```'
    printf '%s\n' "$stable"
    echo '```'
  else
    echo "稳定层没有不兼容变化。"
  fi
  if [ -n "$experimental" ]; then
    echo
    echo "### 实验层的不兼容变化（仅提示）"
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
  echo "::warning::the experimental API has incompatible changes (see job summary)"
fi

if [ -n "$stable" ]; then
  if [ "${ALLOW_API_BREAK:-}" = 1 ]; then
    [ -n "${GITHUB_ACTIONS:-}" ] && echo "::warning::incompatible changes in the stable API (allowed by api-break)"
    echo
    echo "ALLOW_API_BREAK=1: allowed. Describe the change and the migration in CHANGELOG.md."
    exit 0
  fi
  [ -n "${GITHUB_ACTIONS:-}" ] && echo "::error::incompatible changes in the stable API (see job summary)"
  echo
  echo "Incompatible changes in the stable API. Keep the old API (add new"
  echo "functions/options instead), or if the break is intended: label the PR"
  echo "api-break (or put [api-break] in the commit message) and describe the"
  echo "migration in CHANGELOG.md."
  exit 1
fi
