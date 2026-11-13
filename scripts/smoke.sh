#!/usr/bin/env bash
# Runs Tango's smoke programs on this machine and summarizes PASS/FAIL.
# Used both locally and by CI (.github/workflows/ci.yml); see docs/ci.md.
#
#   bash scripts/smoke.sh                 desktop smoke programs of this OS
#   bash scripts/smoke.sh desktop NAME..  only the named programs
#   bash scripts/smoke.sh list            print the desktop program names
#   bash scripts/smoke.sh ios             build examples/mobile-hello, run it in a simulator
#   bash scripts/smoke.sh android         install the mobile-hello APK on the connected
#                                         device / emulator and run it
#
# Each program opens real windows for a few seconds and quits by itself.
# The exit status is non-zero if any program failed, timed out or did not
# build. On a locked macOS screen, a program whose only failures are checks
# that need an unlocked screen (camera permission, accessibility focus,
# display link, Metal device) is recorded as SKIP instead and does not fail
# the run; unlock and rerun to check them. On Windows use scripts/smoke.ps1
# (this script hands over to it).
#
# Environment (all optional):
#   SMOKE_OUT                  logs, screenshots, summary.txt (default build/smoke)
#   SMOKE_TIMEOUT              hard limit per program in seconds (default 120)
#   SMOKE_SCREENSHOT=1         take a screenshot of the screen while each program runs
#   SMOKE_SHOT_DELAY           seconds after start for that screenshot (default 2)
#   TANGO_SMOKE_TIMEOUT_SCALE  stretches the programs' own timeouts (CI uses 3)
#   TANGO_SMOKE_SKIP           check groups notify-demo skips (e.g. notify)
#   SMOKE_IOS_DEVICE           simulator name or UDID (default: first available iPhone)
#   SMOKE_MOBILE_WAIT          seconds to wait for the mobile page (default 90)
#   TANGO_APP_ID               Android applicationId (default: "id" in examples/mobile-hello/tan.json)
#   SMOKE_APK                  APK to install (default: build it with mobile/android/build.sh)
set -uo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo"

out="${SMOKE_OUT:-$repo/build/smoke}"
mkdir -p "$out"
out="$(cd "$out" && pwd)"
limit="${SMOKE_TIMEOUT:-120}"
mode="${1:-desktop}"
[[ $# -gt 0 ]] && shift

results=()
failed=0
skipped=0

say() { printf '==> %s\n' "$*"; }

record() { # record NAME STATUS DETAIL
	results+=("$(printf '%-14s %-8s %s' "$1" "$2" "$3")")
	case "$2" in
	PASS) ;;
	SKIP) skipped=$((skipped + 1)) ;;
	*)
		failed=$((failed + 1))
		annotate "$1" "$2" "$3"
		;;
	esac
}

# annotate NAME STATUS DETAIL: in GitHub Actions, report a failed program
# as an error annotation carrying its FAIL lines. Annotations can be read
# without signing in (unlike job logs), which makes CI failures diagnosable
# from the public API.
annotate() {
	[[ -n "${GITHUB_ACTIONS:-}" ]] || return 0
	local log="$out/$1.log" lines=""
	if [[ -f "$log" ]]; then
		lines="$(grep -E '(^| )(FAIL|panic:)' "$log" | grep -v -E '^FAIL: ' | head -n 8 |
			sed -E 's/^[0-9:. /]+//' | tr '\n' '|')"
	fi
	# Workflow commands need %, CR and LF escaped in the message.
	lines="${lines//'%'/'%25'}"
	echo "::error title=smoke $1 $2::$3 ${lines:+-- $lines}"
}

# screen_locked succeeds when the macOS console session is locked. A locked
# screen pauses display links, makes pages hidden (WebKit defers camera
# requests) and can cost Metal its device, so some checks cannot pass.
screen_locked() {
	[[ "$(uname -s)" == Darwin ]] || return 1
	ioreg -n Root -d1 -a 2>/dev/null |
		plutil -extract IOConsoleUsers xml1 -o - - 2>/dev/null |
		grep -A1 CGSSessionScreenIsLocked | grep -q '<true/>'
}

# lock_sensitive LOG succeeds when every FAIL line in LOG is a check known
# to need an unlocked screen (see screen_locked). Any other failure keeps
# the program failed.
lock_sensitive_re='permission request callback|AXButton|no notification arrived|AXFocused|GPU device lost|display ?link|invalidate|coalesce'
lock_sensitive() {
	local fails
	fails="$(grep -E '(^| )FAIL ' "$1" | grep -v -E '^FAIL: ' || true)"
	[[ -n "$fails" ]] || return 1
	! grep -v -E "$lock_sensitive_re" <<<"$fails" | grep -q .
}

summary() {
	{
		echo "smoke summary ($(uname -s) $mode, $(date -u +%Y-%m-%dT%H:%M:%SZ))"
		for r in "${results[@]+"${results[@]}"}"; do echo "  $r"; done
		if ((failed == 0 && skipped > 0)); then
			echo "PASS ($((${#results[@]} - skipped)) of ${#results[@]}; $skipped skipped: screen locked, unlock and rerun to check them)"
		elif ((failed == 0)); then
			echo "ALL PASS (${#results[@]})"
		else
			echo "FAILED: $failed of ${#results[@]}"
		fi
	} | tee "$out/summary.txt"
	if [[ -n "${GITHUB_STEP_SUMMARY:-}" ]]; then
		{
			echo '```'
			cat "$out/summary.txt"
			echo '```'
		} >>"$GITHUB_STEP_SUMMARY"
	fi
	((failed == 0))
}

# screenshot FILE: whole screen, best effort.
screenshot() {
	case "$(uname -s)" in
	Darwin) screencapture -x "$1" 2>/dev/null ;;
	Linux)
		if [[ -n "${DISPLAY:-}" ]] && command -v import >/dev/null; then
			import -display "$DISPLAY" -window root "$1" 2>/dev/null
		fi
		;;
	esac
}

# run_one NAME BINARY [ARGS...]: runs with a hard timeout, logs to $out/NAME.log.
run_one() {
	local name="$1" bin="$2"
	shift 2
	local log="$out/$name.log" marker="$out/.$name.timeout"
	rm -f "$marker"
	say "run $name $*"
	local t0=$SECONDS
	"$bin" "$@" </dev/null >"$log" 2>&1 &
	local pid=$!
	# Watchdog (and optional screenshot): polls the program instead of
	# sleeping the whole limit, so it ends on its own right after it.
	(
		shot="${SMOKE_SHOT_DELAY:-2}" n=0
		while kill -0 "$pid" 2>/dev/null; do
			if [[ "${SMOKE_SCREENSHOT:-}" == 1 ]] && ((n == shot)); then
				screenshot "$out/$name.png"
			fi
			if ((n >= limit)); then
				touch "$marker"
				kill -TERM "$pid" 2>/dev/null
				sleep 3
				kill -KILL "$pid" 2>/dev/null
				break
			fi
			sleep 1
			n=$((n + 1))
		done
	) </dev/null >/dev/null 2>&1 &
	local wd=$!
	wait "$pid"
	local rc=$?
	wait "$wd" 2>/dev/null
	sed 's/^/    /' "$log"
	local secs=$((SECONDS - t0))
	if [[ -f "$marker" ]]; then
		rm -f "$marker"
		record "$name" TIMEOUT "killed after ${limit}s"
	elif ((rc == 0)); then
		record "$name" PASS "${secs}s"
	elif screen_locked && lock_sensitive "$log"; then
		record "$name" SKIP "screen locked; only lock-sensitive checks failed, see $log"
	else
		record "$name" FAIL "exit $rc after ${secs}s, see $log"
	fi
}

# desktop_programs prints "name|package|args" for this OS.
desktop_programs() {
	case "$(uname -s)" in
	Darwin)
		echo "darwin-smoke|./internal/platform/darwin/internal/smoke|"
		echo "smokeext|./internal/platform/darwin/internal/smokeext|"
		echo "smoketray|./internal/platform/darwin/internal/smoketray|"
		echo "smokecanvas|./internal/platform/darwin/internal/smokecanvas|-smoke"
		echo "a11y-demo|./examples/a11y-demo|-smoke"
		echo "webview-demo|./examples/webview-demo|-smoke"
		echo "sysx-demo|./examples/sysx-demo|-smoke"
		;;
	Linux)
		echo "linux-smoke|./internal/platform/linux/internal/smoke|"
		echo "smokecanvas|./internal/platform/linux/internal/smokecanvas|-smoke"
		;;
	esac
	echo "hello|./examples/hello|-smoke"
	echo "app-demo|./examples/app-demo|-smoke"
	echo "system-demo|./examples/system-demo|"
	echo "notify-demo|./examples/notify-demo|"
}

run_desktop() {
	local want=" $* "
	local bindir="$out/bin"
	mkdir -p "$bindir"
	local name pkg args
	rm -f /tmp/tango-darwin-smoke.png /tmp/tango-smokecanvas*.png # stale copies from earlier runs
	while IFS='|' read -r name pkg args; do
		[[ $# -gt 0 && "$want" != *" $name "* ]] && continue
		say "build $name ($pkg)"
		if go build -o "$bindir/$name" "$pkg" >"$out/$name.build.log" 2>&1; then
			rm -f "$out/$name.build.log"
		else
			sed 's/^/    /' "$out/$name.build.log"
			record "$name" FAIL "build failed, see $out/$name.build.log"
			continue
		fi
		# shellcheck disable=SC2086 # args is a plain word list
		run_one "$name" "$bindir/$name" $args
	done < <(desktop_programs)
	# The darwin smoke programs save their own screenshots in /tmp.
	for f in /tmp/tango-darwin-smoke.png /tmp/tango-smokecanvas*.png; do
		[[ -f "$f" ]] && cp "$f" "$out/" 2>/dev/null
	done
	((${#results[@]} > 0)) || {
		echo "no smoke program matched: $*" >&2
		return 2
	}
}

# wait_for_log FILE PATTERN SECONDS
wait_for_log() {
	local i
	for ((i = 0; i < $3; i++)); do
		grep -q "$2" "$1" 2>/dev/null && return 0
		sleep 1
	done
	return 1
}

run_ios() {
	local wait="${SMOKE_MOBILE_WAIT:-90}" log="$out/ios.log" app udid booted=0
	say "build examples/mobile-hello for the iOS simulator"
	if ! app="$(TANGO_IOS_OUT="$out/ios" bash mobile/ios/build.sh examples/mobile-hello simulator 2>"$out/ios.build.log" | tail -n 1)" || [[ ! -d "$app" ]]; then
		sed 's/^/    /' "$out/ios.build.log"
		record ios FAIL "build failed, see $out/ios.build.log"
		return
	fi
	local id
	id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$app/Info.plist")"
	udid="${SMOKE_IOS_DEVICE:-}"
	if [[ -z "$udid" ]]; then
		udid="$(xcrun simctl list devices available | grep -E '^ +iPhone' | head -n 1 | grep -oE '[0-9A-F-]{36}')"
	elif [[ ! "$udid" =~ ^[0-9A-F-]{36}$ ]]; then
		udid="$(xcrun simctl list devices available | grep -F "    $udid (" | head -n 1 | grep -oE '[0-9A-F-]{36}')"
	fi
	if [[ -z "$udid" ]]; then
		xcrun simctl list devices available
		record ios FAIL "no iPhone simulator available"
		return
	fi
	if ! xcrun simctl list devices | grep -q "$udid) (Booted)"; then
		say "boot simulator $udid"
		xcrun simctl boot "$udid" && booted=1
	fi
	xcrun simctl bootstatus "$udid" -b >/dev/null
	say "install and launch $id"
	xcrun simctl terminate "$udid" "$id" >/dev/null 2>&1
	xcrun simctl install "$udid" "$app" || {
		record ios FAIL "simctl install failed"
		return
	}
	: >"$log"
	# --console-pty relays the app's stdout/stderr (Go's log) until it exits;
	# --stdout/--stderr files stay empty for this host.
	xcrun simctl launch --console-pty --terminate-running-process "$udid" "$id" </dev/null >"$log" 2>&1 &
	local launcher=$!
	if wait_for_log "$log" 'page says' "$wait"; then
		sleep 1
		record ios PASS "page loaded and talked to Go"
	else
		record ios FAIL "no message from the page within ${wait}s, see $log"
	fi
	xcrun simctl io "$udid" screenshot "$out/ios.png" >/dev/null 2>&1
	sed 's/^/    /' "$log"
	xcrun simctl terminate "$udid" "$id" >/dev/null 2>&1
	sleep 1
	kill "$launcher" 2>/dev/null
	wait "$launcher" 2>/dev/null
	((booted)) && xcrun simctl shutdown "$udid" >/dev/null 2>&1
	return 0
}

# adb runs the SDK's adb: from PATH, else $ANDROID_HOME / $ANDROID_SDK_ROOT
# / the default macOS SDK location.
adb() {
	local bin
	bin="$(command -v adb || true)"
	if [[ -z "$bin" ]]; then
		for d in "${ANDROID_HOME:-}" "${ANDROID_SDK_ROOT:-}" "$HOME/Library/Android/sdk" "$HOME/Android/Sdk"; do
			if [[ -n "$d" && -x "$d/platform-tools/adb" ]]; then
				bin="$d/platform-tools/adb"
				break
			fi
		done
	fi
	[[ -n "$bin" ]] || {
		echo "adb not found (install platform-tools or set ANDROID_HOME)" >&2
		return 127
	}
	"$bin" "$@"
}

run_android() {
	local wait="${SMOKE_MOBILE_WAIT:-90}" log="$out/android.log" app=examples/mobile-hello
	local id="${TANGO_APP_ID:-}" apk="${SMOKE_APK:-}"
	if [[ -z "$id" ]]; then # applicationId is the "id" in tan.json
		id="$(sed -n 's/^  "id": *"\([^"]*\)".*/\1/p' "$app/tan.json" | head -n 1)"
	fi
	if [[ -z "$apk" ]]; then
		say "build $app APK"
		if ! bash mobile/android/build.sh "$app" >"$out/android.build.log" 2>&1; then
			sed 's/^/    /' "$out/android.build.log"
			record android FAIL "build failed, see $out/android.build.log"
			return
		fi
		apk="$(sed -n 's/^APK: //p' "$out/android.build.log" | tail -n 1)"
	fi
	adb wait-for-device
	say "install $apk ($id)"
	adb install -r "$apk" || {
		record android FAIL "adb install failed"
		return
	}
	adb logcat -c
	# Start the launcher activity with am: monkey quits without launching
	# anything on emulators that have no hardware keys ("SYS_KEYS has no
	# physical keys"), e.g. API 36 AVDs.
	local activity
	activity="$(adb shell cmd package resolve-activity --brief -c android.intent.category.LAUNCHER "$id" 2>/dev/null | tr -d '\r' | tail -n 1)"
	[[ "$activity" == */* ]] || activity="$id/org.tango.host.TangoActivity"
	adb shell am start -n "$activity" >/dev/null
	local i ok=0
	for ((i = 0; i < wait; i++)); do
		if adb logcat -d -s Tango:V 2>/dev/null | grep -q 'page says'; then
			ok=1
			break
		fi
		sleep 1
	done
	adb logcat -d >"$log" 2>&1
	adb exec-out screencap -p >"$out/android.png" 2>/dev/null
	grep -E ' (Tango|chromium|AndroidRuntime) ' "$log" | sed 's/^/    /'
	if ((ok)); then
		record android PASS "page loaded and talked to Go"
	else
		record android FAIL "no message from the page within ${wait}s, see $log"
	fi
	adb shell am force-stop "$id"
	return 0
}

case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*)
	exec powershell -NoProfile -ExecutionPolicy Bypass -File "$repo/scripts/smoke.ps1" "$mode" "$@"
	;;
esac

case "$mode" in
desktop) run_desktop "$@" || exit $? ;;
list)
	desktop_programs | cut -d'|' -f1
	exit 0
	;;
ios) run_ios ;;
android) run_android ;;
-h | --help | help)
	sed -n '2,30p' "$0" | sed 's/^# \{0,1\}//'
	exit 0
	;;
*)
	echo "smoke.sh: unknown mode '$mode' (desktop, list, ios, android)" >&2
	exit 2
	;;
esac
summary
