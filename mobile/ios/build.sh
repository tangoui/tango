#!/usr/bin/env bash
# Builds a Tango app for iOS. A thin wrapper around tan build, kept for CI
# and existing instructions; the build itself (c-archive, link with main.m,
# Info.plist from tan.json, icon, signing, .ipa) lives in cmd/tan.
#
# Usage:
#   bash mobile/ios/build.sh <app-dir> [simulator|device]
#
# <app-dir> holds tan.json (name, id, version, "ios" settings).
#
# Environment (optional):
#   TANGO_IOS_OUT            output directory (default <app-dir>/build); the
#                            .app is in <out>/ios-simulator or <out>/ios
#   TANGO_IOS_SIGN_IDENTITY  codesign identity for device builds (else
#                            "ios.signIdentity"; the provisioning profile is
#                            "ios.provisioningProfile" in tan.json)
#   TANGO_DEBUG=1            keep symbols (tan build -debug)
#   GOFLAGS                  passed through to go build
#
# Prints the path of the .app on the last line.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"

if [[ $# -lt 1 ]]; then
	sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
	exit 2
fi
appdir="$(cd "$1" && pwd)"
case "${2:-simulator}" in
simulator) platform=ios-simulator ;;
device) platform=ios ;;
*)
	echo "build.sh: platform must be simulator or device, got '$2'" >&2
	exit 2
	;;
esac
out="${TANGO_IOS_OUT:-$appdir/build}"
flags=()
[[ "${TANGO_DEBUG:-}" == 1 ]] && flags+=(-debug)

(cd "$repo/cmd/tan" && go run . build -C "$appdir" -platform "$platform" -o "$out" "${flags[@]+"${flags[@]}"}") >&2
ls -d "$out/$platform"/*.app | head -n 1
