#!/usr/bin/env bash
# Builds a Tango app for Android. A thin wrapper around tan build, kept for
# CI; the build itself (libtango.so per ABI with the NDK, the Gradle project
# with manifest/icon/signing from tan.json, Gradle --offline) is in cmd/tan.
#
#   mobile/android/build.sh [APP_DIR]      (default: examples/mobile-hello)
#
# APP_DIR holds tan.json (name, id, version, "android" settings).
#
# Environment:
#   ANDROID_HOME        Android SDK (default ~/Library/Android/sdk or ~/Android/Sdk)
#   ANDROID_NDK_HOME    NDK (default: newest under $ANDROID_HOME/ndk)
#   TANGO_ABIS          ABIs to build (default "arm64-v8a x86_64")
#   TANGO_BUILD         debug | release (default debug); release needs
#                       "android.keystore"/"keyAlias" in tan.json and
#                       $TANGO_ANDROID_KEYSTORE_PASSWORD
#   TANGO_GRADLE_ARGS   Gradle arguments before the tasks (default --offline)
#   JAVA_HOME           a JDK 17+ for Gradle
#
# Output: <APP_DIR>/build/android/<Name>-<version>-debug.apk (release:
# <Name>-<version>.apk and .aab). The last line is "APK: <path>".
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo="$(cd "$here/../.." && pwd)"
pkg="$(cd "${1:-$repo/examples/mobile-hello}" && pwd)"

platforms=()
for abi in ${TANGO_ABIS:-arm64-v8a x86_64}; do
	platforms+=("android/$abi")
done
platform="$(IFS=,; echo "${platforms[*]}")"
flags=()
[[ "${TANGO_BUILD:-debug}" == release ]] || flags+=(-debug)

(cd "$repo/cmd/tan" && go run . build -C "$pkg" -platform "$platform" "${flags[@]+"${flags[@]}"}")

apk="$(ls -t "$pkg"/build/android/*.apk | head -n 1)"
echo "APK: $apk"
