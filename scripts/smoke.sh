#!/usr/bin/env bash
# Runs Tango's smoke programs on this machine and summarizes PASS/FAIL.
# Used both locally and by CI (.github/workflows/ci.yml); see docs/ci.md.
#
#   bash scripts/smoke.sh                 desktop smoke programs of this OS
#   bash scripts/smoke.sh desktop NAME..  only the named programs
#   bash scripts/smoke.sh list            print the desktop program names
#   bash scripts/smoke.sh ios             build examples/mobile-hello, run it in a simulator
#   bash scripts/smoke.sh ios-probe       build examples/duplex-ios, measure in a simulator
#                                         whether WebKit streams scheme responses
#   bash scripts/smoke.sh ios-selftest    build examples/duplex-ios, run its tango://
#                                         scheme-handler self-test in a simulator
#   bash scripts/smoke.sh android         install the mobile-hello APK on the connected
#                                         device / emulator and run it
#   bash scripts/smoke.sh ios-ui          self-drawn examples/mobile-ui in a simulator:
#                                         first frame, screenshot, self-test taps
#   bash scripts/smoke.sh android-ui      self-drawn examples/mobile-ui on the device /
#                                         emulator: first frame, screenshot, adb tap
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
#   SMOKE_MOBILE_WAIT          seconds to wait for the mobile page (default 90; ios-selftest 180)
#   TANGO_APP_ID               Android applicationId for "android" (default: "id" in
#                              examples/mobile-hello/tan.json)
#   SMOKE_APK                  APK to install for "android" (default: build it with
#                              mobile/android/build.sh into $SMOKE_OUT/bin/android)
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
		lines="$(grep -E '(^| )(FAIL|panic:)' "$log" | head -n 8 |
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

# ios_build NAME APP_DIR: builds APP_DIR for the simulator into $out/NAME;
# sets ios_app (the .app) and ios_id (its bundle ID).
ios_build() {
	say "build $2 for the iOS simulator"
	if ! ios_app="$(TANGO_IOS_OUT="$out/$1" bash mobile/ios/build.sh "$2" simulator 2>"$out/$1.build.log" | tail -n 1)" || [[ ! -d "$ios_app" ]]; then
		sed 's/^/    /' "$out/$1.build.log"
		record "$1" FAIL "build failed, see $out/$1.build.log"
		return 1
	fi
	ios_id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$ios_app/Info.plist")"
}

# ios_boot NAME: picks the simulator (SMOKE_IOS_DEVICE or the first
# iPhone) and boots it; sets ios_udid, and ios_booted=1 when this script
# booted it (ios_done shuts it down again).
ios_boot() {
	ios_booted=0
	ios_udid="${SMOKE_IOS_DEVICE:-}"
	if [[ -z "$ios_udid" ]]; then
		ios_udid="$(xcrun simctl list devices available | grep -E '^ +iPhone' | head -n 1 | grep -oE '[0-9A-F-]{36}')"
	elif [[ ! "$ios_udid" =~ ^[0-9A-F-]{36}$ ]]; then
		ios_udid="$(xcrun simctl list devices available | grep -F "    $ios_udid (" | head -n 1 | grep -oE '[0-9A-F-]{36}')"
	fi
	if [[ -z "$ios_udid" ]]; then
		xcrun simctl list devices available
		record "$1" FAIL "no iPhone simulator available"
		return 1
	fi
	if ! xcrun simctl list devices | grep -q "$ios_udid) (Booted)"; then
		say "boot simulator $ios_udid"
		xcrun simctl boot "$ios_udid" && ios_booted=1
	fi
	xcrun simctl bootstatus "$ios_udid" -b >/dev/null
}

# ios_launch NAME LOG [VAR=VALUE...]: installs ios_app and launches it in
# the background with its output in LOG; the VARs go into the app's
# environment. Sets ios_launcher.
ios_launch() {
	local name="$1" log="$2" kv
	shift 2
	say "install and launch $ios_id"
	xcrun simctl terminate "$ios_udid" "$ios_id" >/dev/null 2>&1
	xcrun simctl install "$ios_udid" "$ios_app" || {
		record "$name" FAIL "simctl install failed"
		return 1
	}
	: >"$log"
	# simctl passes SIMCTL_CHILD_X to the app as X.
	local childenv=()
	for kv in "$@"; do childenv+=("SIMCTL_CHILD_$kv"); done
	# --console-pty relays the app's stdout/stderr (Go's log) until it exits;
	# --stdout/--stderr files stay empty for this host.
	env "${childenv[@]+"${childenv[@]}"}" xcrun simctl launch --console-pty --terminate-running-process "$ios_udid" "$ios_id" </dev/null >"$log" 2>&1 &
	ios_launcher=$!
}

# ios_done LOG: prints LOG, stops the app and shuts down a simulator this
# script booted.
ios_done() {
	sed 's/^/    /' "$1"
	xcrun simctl terminate "$ios_udid" "$ios_id" >/dev/null 2>&1
	sleep 1
	kill "$ios_launcher" 2>/dev/null
	wait "$ios_launcher" 2>/dev/null
	((ios_booted)) && xcrun simctl shutdown "$ios_udid" >/dev/null 2>&1
	return 0
}

run_ios() {
	local wait="${SMOKE_MOBILE_WAIT:-90}" log="$out/ios.log"
	ios_build ios examples/mobile-hello || return 0
	ios_boot ios || return 0
	ios_launch ios "$log" || { ios_done "$log"; return 0; }
	if wait_for_log "$log" 'page says' "$wait"; then
		sleep 1
		record ios PASS "page loaded and talked to Go"
	else
		record ios FAIL "no message from the page within ${wait}s, see $log"
	fi
	xcrun simctl io "$ios_udid" screenshot "$out/ios.png" >/dev/null 2>&1
	ios_done "$log"
}

# run_ios_ui: the self-drawn examples/mobile-ui. simctl cannot tap, so the
# app's own self-test (examples/mobile-ui/selftest_ios.go) feeds scripted
# touches to the canvas view; the counter row's log line proves that they
# went through the canvas, the runner and the widgets.
run_ios_ui() {
	local wait="${SMOKE_MOBILE_WAIT:-90}" log="$out/ios-ui.log"
	ios_build ios-ui examples/mobile-ui || return 0
	ios_boot ios-ui || return 0
	ios_launch ios-ui "$log" TANGO_MOBILEUI_SELFTEST=1 TANGO_MOBILEUI_SELFTEST_STAY=1 || {
		ios_done "$log"
		return 0
	}
	if ! wait_for_log "$log" 'mobile-ui: first frame' "$wait"; then
		record ios-ui FAIL "no first frame within ${wait}s, see $log"
	else
		sleep 1
		xcrun simctl io "$ios_udid" screenshot "$out/ios-ui.png" >/dev/null 2>&1
		if wait_for_log "$log" 'mobile-ui selftest: done' 30 && grep -q 'mobile-ui: tap #1' "$log"; then
			record ios-ui PASS "first frame; self-test tap reached the counter row"
		else
			record ios-ui FAIL "first frame, but the self-test tap did not reach the counter row, see $log"
		fi
		xcrun simctl io "$ios_udid" screenshot "$out/ios-ui-tap.png" >/dev/null 2>&1
	fi
	ios_done "$log"
}

# run_ios_probe measures whether WKWebView hands a WKURLSchemeTask's
# separate didReceiveData: calls to the page's fetch() ReadableStream as
# they happen. examples/duplex-ios runs with TANGO_SCHEME_PROBE=1: the iOS
# backend answers /__tango/probe/stream itself with 8 chunks of 1 KiB, 200 ms
# apart, and the page logs "probe: {...}" with each chunk's arrival time.
# PASS (incremental delivery) needs status 200, all 8192 bytes, at least 4
# chunks (WebKit may merge neighbours), and the first chunk more than
# 1000 ms before the last.
run_ios_probe() {
	local wait="${SMOKE_MOBILE_WAIT:-90}" log="$out/ios-probe.log" app udid booted=0
	say "build examples/duplex-ios for the iOS simulator"
	if ! app="$(GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=tango_selftest" TANGO_IOS_OUT="$out/ios-probe" bash mobile/ios/build.sh examples/duplex-ios simulator 2>"$out/ios-probe.build.log" | tail -n 1)" || [[ ! -d "$app" ]]; then
		sed 's/^/    /' "$out/ios-probe.build.log"
		record ios-probe FAIL "build failed, see $out/ios-probe.build.log"
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
		record ios-probe FAIL "no iPhone simulator available"
		return
	fi
	if ! xcrun simctl list devices | grep -q "$udid) (Booted)"; then
		say "boot simulator $udid"
		xcrun simctl boot "$udid" && booted=1
	fi
	xcrun simctl bootstatus "$udid" -b >/dev/null
	say "install and launch $id with TANGO_SCHEME_PROBE=1"
	xcrun simctl terminate "$udid" "$id" >/dev/null 2>&1
	if ! xcrun simctl install "$udid" "$app"; then
		record ios-probe FAIL "simctl install failed"
		((booted)) && xcrun simctl shutdown "$udid" >/dev/null 2>&1
		return
	fi
	: >"$log"
	# simctl passes SIMCTL_CHILD_X=v to the app as X=v.
	SIMCTL_CHILD_TANGO_SCHEME_PROBE=1 xcrun simctl launch --console-pty --terminate-running-process "$udid" "$id" </dev/null >"$log" 2>&1 &
	local launcher=$!
	if wait_for_log "$log" 'probe: ' "$wait"; then
		sleep 1
		local line ts n first last bytes status
		line="$(grep -m 1 -oE 'probe: \{.*\}' "$log")"
		ts="$(grep -oE '"t":[0-9]+' <<<"$line" | cut -d: -f2)"
		n="$(grep -c . <<<"$ts")"
		first="$(head -n 1 <<<"$ts")"
		last="$(tail -n 1 <<<"$ts")"
		bytes="$(grep -oE '"bytes":[0-9]+' <<<"$line" | cut -d: -f2)"
		status="$(grep -oE '"status":[0-9]+' <<<"$line" | cut -d: -f2)"
		if [[ "$status" != 200 || "$bytes" != 8192 ]]; then
			record ios-probe FAIL "probe fetch: status ${status:-?}, ${bytes:-?} bytes"
		elif ((n >= 4 && first < last - 1000)); then
			record ios-probe PASS "incremental: $n chunks, first at $first ms, last at $last ms"
		else
			record ios-probe FAIL "not incremental: $n chunks, first at ${first:-?} ms, last at ${last:-?} ms"
		fi
	else
		record ios-probe FAIL "no probe result within ${wait}s, see $log"
	fi
	xcrun simctl io "$udid" screenshot "$out/ios-probe.png" >/dev/null 2>&1
	sed 's/^/    /' "$log"
	xcrun simctl terminate "$udid" "$id" >/dev/null 2>&1
	sleep 1
	kill "$launcher" 2>/dev/null
	wait "$launcher" 2>/dev/null
	((booted)) && xcrun simctl shutdown "$udid" >/dev/null 2>&1
	return 0
}

# run_ios_selftest runs the tango:// scheme-handler self-test of
# examples/duplex-ios (TANGO_SCHEME_PROBE=1 for the /__test routes and
# Debug.Stats, TANGO_SCHEME_SELFTEST=1 for index.html?selftest): the request
# sequence of the macOS parity test (index, 3 MiB, 302, HEAD, 64 KiB POST
# echo, panic) checked against the expected answers, an aborted
# never-ending stream whose Go handler must return, the fast lanes the page
# got (TANGO_IPC_LANES_DARWIN=1, needed on iOS where lanes are off by
# default: tango.lanes() must be fallback / up:post,
# and a 1 MiB Echo must use up:post), the lane scenarios of
# examples/duplex/services/lanetests.js (a reload during an up:post upload
# with no leak, 8 MiB both ways, mixed sizes in order, a 6 s main-thread
# stall; the page reloads itself once for the first), and an ipc Echo. The
# page logs "selftest: {...}". PASS needs a top-level "ok":true, the index
# body equal to web/index.html, and no WebKit exception in the log. The lane
# fault scenario needs a tango_dev build, which tan build does not make; it
# is not run here. (The other hooks work without tango_dev because the iOS
# backend honours TANGO_SCHEME_PROBE in release builds on purpose; on macOS
# they need -tags tango_dev. See docs/dev-builds.md.)
run_ios_selftest() {
	local wait="${SMOKE_MOBILE_WAIT:-180}" log="$out/ios-selftest.log" app udid booted=0
	say "build examples/duplex-ios for the iOS simulator"
	if ! app="$(GOFLAGS="${GOFLAGS:+$GOFLAGS }-tags=tango_selftest" TANGO_IOS_OUT="$out/ios-selftest" bash mobile/ios/build.sh examples/duplex-ios simulator 2>"$out/ios-selftest.build.log" | tail -n 1)" || [[ ! -d "$app" ]]; then
		sed 's/^/    /' "$out/ios-selftest.build.log"
		record ios-selftest FAIL "build failed, see $out/ios-selftest.build.log"
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
		record ios-selftest FAIL "no iPhone simulator available"
		return
	fi
	if ! xcrun simctl list devices | grep -q "$udid) (Booted)"; then
		say "boot simulator $udid"
		xcrun simctl boot "$udid" && booted=1
	fi
	xcrun simctl bootstatus "$udid" -b >/dev/null
	say "install and launch $id with TANGO_SCHEME_PROBE=1 TANGO_SCHEME_SELFTEST=1 TANGO_IPC_LANES_DARWIN=1"
	xcrun simctl terminate "$udid" "$id" >/dev/null 2>&1
	if ! xcrun simctl install "$udid" "$app"; then
		record ios-selftest FAIL "simctl install failed"
		((booted)) && xcrun simctl shutdown "$udid" >/dev/null 2>&1
		return
	fi
	: >"$log"
	# simctl passes SIMCTL_CHILD_X=v to the app as X=v.
	SIMCTL_CHILD_TANGO_SCHEME_PROBE=1 SIMCTL_CHILD_TANGO_SCHEME_SELFTEST=1 SIMCTL_CHILD_TANGO_IPC_LANES_DARWIN=1 \
		xcrun simctl launch --console-pty --terminate-running-process "$udid" "$id" </dev/null >"$log" 2>&1 &
	local launcher=$!
	if wait_for_log "$log" 'selftest: ' "$wait"; then
		sleep 1
		local line sha want bytes wantBytes failed stopWait
		line="$(grep -m 1 -oE 'selftest: \{.*\}' "$log")"
		sha="$(grep -oE '"indexSha":"[0-9a-f]+"' <<<"$line" | cut -d'"' -f4)"
		bytes="$(grep -oE '"indexBytes":[0-9]+' <<<"$line" | cut -d: -f2)"
		want="$(shasum -a 256 examples/duplex-ios/web/index.html | cut -d' ' -f1)"
		wantBytes="$(wc -c <examples/duplex-ios/web/index.html | tr -d ' ')"
		failed="$(grep -oE '"failed":\[[^]]*\]' <<<"$line")"
		stopWait="$(grep -oE '"wait":[0-9]+' <<<"$line" | cut -d: -f2)"
		if grep -q 'NSInternalInconsistencyException' "$log"; then
			record ios-selftest FAIL "WebKit exception, see $log"
		elif ! grep -q '^selftest: {"ok":true,' <<<"$line"; then
			record ios-selftest FAIL "checks failed: ${failed:-?}"
		elif [[ "$sha" != "$want" || "$bytes" != "$wantBytes" ]]; then
			record ios-selftest FAIL "index body: $bytes bytes sha ${sha:-?}, want $wantBytes bytes sha $want"
		else
			record ios-selftest PASS "parity, stop (handler returned ${stopWait:-?} ms after abort), lanes (up:post echo), lane scenarios (reload, 8 MiB, mixed, stall) and echo ok"
		fi
	else
		record ios-selftest FAIL "no selftest result within ${wait}s, see $log"
	fi
	xcrun simctl io "$udid" screenshot "$out/ios-selftest.png" >/dev/null 2>&1
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
	bin="$(type -P adb || true)" # not command -v: that finds this function
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

# android_start NAME APP_DIR [APK]: builds APP_DIR's APK into $out/bin/NAME
# (unless APK is given), installs it and starts its launcher activity with
# a clear logcat. Sets android_id (the applicationId).
android_start() {
	local name="$1" app="$2" apk="${3:-}"
	android_id="${TANGO_APP_ID:-}"
	if [[ -z "$android_id" || "$name" != android ]]; then # applicationId is the "id" in tan.json
		android_id="$(sed -n 's/^  "id": *"\([^"]*\)".*/\1/p' "$app/tan.json" | head -n 1)"
	fi
	if [[ -z "$apk" ]]; then
		say "build $app APK"
		if ! TANGO_ANDROID_OUT="$out/bin/$name" bash mobile/android/build.sh "$app" >"$out/$name.build.log" 2>&1; then
			sed 's/^/    /' "$out/$name.build.log"
			record "$name" FAIL "build failed, see $out/$name.build.log"
			return 1
		fi
		apk="$(sed -n 's/^APK: //p' "$out/$name.build.log" | tail -n 1)"
	fi
	adb wait-for-device
	say "install $apk ($android_id)"
	adb install -r "$apk" || {
		record "$name" FAIL "adb install failed"
		return 1
	}
	adb shell am force-stop "$android_id"
	adb logcat -c
	# Start the launcher activity with am: monkey quits without launching
	# anything on emulators that have no hardware keys ("SYS_KEYS has no
	# physical keys"), e.g. API 36 AVDs.
	local activity
	activity="$(adb shell cmd package resolve-activity --brief -c android.intent.category.LAUNCHER "$android_id" 2>/dev/null | tr -d '\r' | tail -n 1)"
	[[ "$activity" == */* ]] || activity="$android_id/org.tango.host.TangoActivity"
	adb shell am start -n "$activity" >/dev/null
}

# android_wait PATTERN SECONDS: waits for PATTERN in the Tango log.
android_wait() {
	local i
	for ((i = 0; i < $2; i++)); do
		adb logcat -d -s Tango:V 2>/dev/null | grep -q "$1" && return 0
		sleep 1
	done
	return 1
}

# android_log NAME: saves logcat to $out/NAME.log and prints the app's lines.
android_log() {
	adb logcat -d >"$out/$1.log" 2>&1
	grep -E ' (Tango|chromium|AndroidRuntime) ' "$out/$1.log" | sed 's/^/    /'
}

run_android() {
	local wait="${SMOKE_MOBILE_WAIT:-90}" ok=0
	android_start android examples/mobile-hello "${SMOKE_APK:-}" || return 0
	android_wait 'page says' "$wait" && ok=1
	android_log android
	adb exec-out screencap -p >"$out/android.png" 2>/dev/null
	if ((ok)); then
		record android PASS "page loaded and talked to Go"
	else
		record android FAIL "no message from the page within ${wait}s, see $out/android.log"
	fi
	adb shell am force-stop "$android_id"
	return 0
}

# android_bounds TEXT: prints the centre "x y" of the first node whose
# text or content-desc contains TEXT in a uiautomator dump of the screen.
# The canvas exposes its widgets through its AccessibilityNodeProvider.
android_bounds() {
	adb shell uiautomator dump /sdcard/tango-smoke.xml >/dev/null 2>&1 || return 1
	adb shell cat /sdcard/tango-smoke.xml 2>/dev/null | tr '>' '\n' |
		grep -F -e "text=\"$1" -e "content-desc=\"$1" | head -n 1 |
		sed -n 's/.*bounds="\[\([0-9]*\),\([0-9]*\)\]\[\([0-9]*\),\([0-9]*\)\]".*/\1 \2 \3 \4/p' |
		awk '{ printf "%d %d\n", ($1 + $3) / 2, ($2 + $4) / 2 }'
}

# run_android_ui: the self-drawn examples/mobile-ui. adb input tap injects
# a real MotionEvent on the "点我计数" row (found through the canvas's
# accessibility tree); its log line proves that the touch went through the
# view, the runner and the widgets.
run_android_ui() {
	local wait="${SMOKE_MOBILE_WAIT:-90}" name=android-ui at prop
	# Log the canvas's input events too (read when the app starts); the
	# previous value is put back afterwards.
	adb wait-for-device
	prop="$(adb shell getprop debug.tango.canvas | tr -d '\r')"
	adb shell setprop debug.tango.canvas 1
	android_start "$name" examples/mobile-ui || {
		adb shell setprop debug.tango.canvas "${prop:-0}"
		return 0
	}
	if ! android_wait 'mobile-ui: first frame' "$wait"; then
		android_log "$name"
		record "$name" FAIL "no first frame within ${wait}s, see $out/$name.log"
	else
		sleep 1
		adb exec-out screencap -p >"$out/$name.png" 2>/dev/null
		at="$(android_bounds 点我计数)"
		if [[ -z "$at" ]]; then
			android_log "$name"
			record "$name" FAIL "no \"点我计数\" row in the accessibility tree, see $out/$name.log"
		else
			say "tap 点我计数 at $at"
			# shellcheck disable=SC2086 # at is "x y"
			adb shell input tap $at
			if android_wait 'mobile-ui: tap #1' 10; then
				record "$name" PASS "first frame; adb tap reached the counter row"
			else
				record "$name" FAIL "the tap at $at did not reach the counter row, see $out/$name.log"
			fi
			sleep 1
			adb exec-out screencap -p >"$out/$name-tap.png" 2>/dev/null
			android_log "$name"
		fi
	fi
	adb shell am force-stop "$android_id"
	adb shell setprop debug.tango.canvas "${prop:-0}"
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
ios-ui) run_ios_ui ;;
ios-probe) run_ios_probe ;;
ios-selftest) run_ios_selftest ;;
android) run_android ;;
android-ui) run_android_ui ;;
-h | --help | help)
	sed -n '2,/^set /p' "$0" | sed -e '$d' -e 's/^# \{0,1\}//'
	exit 0
	;;
*)
	echo "smoke.sh: unknown mode '$mode' (desktop, list, ios, ios-ui, ios-probe, ios-selftest, android, android-ui)" >&2
	exit 2
	;;
esac
summary
