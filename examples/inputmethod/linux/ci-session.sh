#!/usr/bin/env bash
set -Eeuo pipefail
trap 'printf "Smoke failure at line %s: %s\n" "$LINENO" "$BASH_COMMAND" >&2' ERR

framework=${1:?ibus or fcitx}
out=${2:?evidence directory}
here=$(cd "$(dirname "$0")" && pwd)
engine=com.tangoui.inputmethod.example
expect=${3:-installed}
log_prefix="$out/$framework"
if [[ "$expect" != installed ]]; then log_prefix="$log_prefix-$expect"; fi
daemon_log="$log_prefix-daemon.log"
session=$(mktemp -d)
pids=()
cleanup() {
    for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
    wait 2>/dev/null || true
    # Some runners mount an XDG document portal inside this session.
    if command -v fusermount3 >/dev/null && [[ -d "$session/runtime/doc" ]]; then
        fusermount3 -u "$session/runtime/doc" 2>/dev/null || true
    fi
    rm -rf -- "$session" 2>/dev/null || true
}
trap cleanup EXIT

export XDG_CONFIG_HOME="$session/config" XDG_DATA_HOME="$session/data"
export XDG_RUNTIME_DIR="$session/runtime" TANGO_IME_SMOKE_TRACE=1
mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME"
mkdir -m 700 "$XDG_RUNTIME_DIR"
# CI runs these sessions sequentially on a disposable job runner.
export DISPLAY=:91
Xvfb "$DISPLAY" -screen 0 800x600x24 -nolisten tcp -noreset >"$log_prefix-xvfb.log" 2>&1 &
xvfb_pid=$!
pids+=("$xvfb_pid")
display_ready=false
for i in {1..150}; do
    kill -0 "$xvfb_pid"
    if xwininfo -root >/dev/null 2>&1; then
        display_ready=true
        break
    fi
    sleep 0.1
done
test "$display_ready" = true
export GDK_BACKEND=x11 GTK_IM_MODULE="$framework" XMODIFIERS="@im=$framework"
dbus-update-activation-environment DISPLAY GTK_IM_MODULE XMODIFIERS XDG_CONFIG_HOME XDG_DATA_HOME XDG_RUNTIME_DIR TANGO_IME_SMOKE_TRACE

if [[ "$framework" == ibus ]]; then
    export TANGO_IME_IBUS_TRACE="$out/ibus-panel.log"
    if [[ "$expect" == installed ]]; then : > "$TANGO_IME_IBUS_TRACE"; fi
    ibus-daemon --xim --panel=disable --config=disable >"$daemon_log" 2>&1 &
    pids+=("$!")
    for i in {1..50}; do
        if ibus list-engine >"$session/engine-list" 2>/dev/null; then
            if [[ "$expect" == removed ]] || grep -Fq "$engine" "$session/engine-list"; then break; fi
        fi
        sleep 0.2
    done
    if [[ "$expect" == removed ]]; then
        ibus list-engine >"$out/ibus-after-removal.txt"
        if grep -Fq "$engine" "$out/ibus-after-removal.txt"; then exit 1; fi
        exit 0
    fi
else
    mkdir -p "$XDG_CONFIG_HOME/fcitx5"
    # Test the default shortcut in the panel probe. For the GTK test,
    # reserve both standalone Shift keys for Tango.
    if [[ "$expect" == installed ]]; then
        cat >"$XDG_CONFIG_HOME/fcitx5/config" <<'CONFIG'
[Hotkey/AltTriggerKeys]
0=Control+Shift_L
CONFIG
    fi
    cat >"$XDG_CONFIG_HOME/fcitx5/profile" <<PROFILE
[Groups/0]
Name=Default
Default Layout=us
DefaultIM=$engine
[Groups/0/Items/0]
Name=keyboard-us
Layout=
[Groups/0/Items/1]
Name=$engine
Layout=
[GroupOrder]
0=Default
PROFILE
    fcitx5 --replace >"$daemon_log" 2>&1 &
    pids+=("$!")
    # Poll the bus so probing readiness cannot activate a second daemon.
    for i in {1..50}; do
        owner=$(gdbus call --session --dest org.freedesktop.DBus --object-path /org/freedesktop/DBus \
            --method org.freedesktop.DBus.NameHasOwner org.fcitx.Fcitx5)
        [[ "$owner" == '(true,)' ]] && break
        sleep 0.2
    done
    test "$owner" = '(true,)'
    if [[ "$expect" == removed ]]; then
        gdbus call --session --dest org.fcitx.Fcitx5 --object-path /controller \
            --method org.fcitx.Fcitx.Controller1.AvailableInputMethods >"$out/fcitx-after-removal.txt"
        if grep -Fq "$engine" "$out/fcitx-after-removal.txt"; then exit 1; fi
        exit 0
    fi
    fcitx5-remote -s "$engine"
    if [[ "$expect" == panel-preedit ]]; then
        /usr/bin/python3 "$here/fcitx-panel-smoke.py" | tee "$out/fcitx-panel-focus.txt"
        test "$(grep -Fc 'standalone left Shift is reserved by Fcitx5' "$daemon_log")" = 1
        exit 0
    fi
fi

export TANGO_SMOKE_TEXT="$out/$framework-text.txt"
: > "$TANGO_SMOKE_TEXT"
focus_file="${TANGO_SMOKE_TEXT%.txt}.focus"
rm -f "$focus_file"
/usr/bin/python3 "$here/gtk-smoke.py" >"$out/$framework-client.log" 2>&1 &
pids+=("$!")
for i in {1..100}; do
    if window=$(xdotool search --onlyvisible --name '^Tango IME Smoke$' 2>/dev/null); then break; fi
    sleep 0.1
done
test -n "$window"
xdotool windowfocus --sync "$window"
for i in {1..50}; do
    [[ -s "$focus_file" ]] && break
    sleep 0.1
done
test "$(cat "$focus_file")" = primary
if [[ "$framework" == ibus ]]; then
    for i in {1..25}; do
        if ibus engine "$engine" && [[ "$(ibus engine 2>/dev/null)" == "$engine" ]]; then break; fi
        sleep 0.2
    done
    test "$(ibus engine)" = "$engine"
    for i in {1..50}; do
        if grep -Fxq 'mode zh' "$TANGO_IME_IBUS_TRACE"; then break; fi
        sleep 0.2
    done
    grep -Fxq 'mode zh' "$TANGO_IME_IBUS_TRACE"
else
    fcitx5-remote -o
    fcitx5-remote -s "$engine"
    # F12 leaves the entry empty while the GTK input context and host connect.
    for i in {1..50}; do
        if grep -Fq 'Tango Fcitx5 mode zh' "$daemon_log"; then break; fi
        xdotool key F12
        sleep 0.2
    done
    grep -Fq 'Tango Fcitx5 mode zh' "$daemon_log"
fi
for key in n i h a o space; do
    xdotool key "$key"
    sleep 0.1
done
sleep 0.3
test "$(cat "$TANGO_SMOKE_TEXT")" = 你好
# Numeric X11 keycodes avoid xdotool synthesizing an extra left Shift to set
# the modifier mask for a named right Shift. Read the keycodes from Xvfb.
shift_codes=$(xkbcomp -xkb "$DISPLAY" - 2>/dev/null)
left_shift=$(awk '$1 == "<LFSH>" { print $3 + 0 }' <<<"$shift_codes")
right_shift=$(awk '$1 == "<RTSH>" { print $3 + 0 }' <<<"$shift_codes")
test -n "$left_shift" && test -n "$right_shift"
xdotool key "$left_shift"
sleep 0.2
for key in a b c; do xdotool key "$key"; done
sleep 0.2
test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc
if [[ "$framework" == fcitx ]]; then
    test "$(fcitx5-remote)" = 2
    test "$(fcitx5-remote -n)" = "$engine"
fi
xdotool key "$right_shift"
sleep 0.2
for key in n i space; do
    xdotool key "$key"
    sleep 0.1
done
sleep 0.2
test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc你

if [[ "$framework" == ibus ]]; then
    grep -Eq '^candidates [1-9][0-9]*$' "$TANGO_IME_IBUS_TRACE"
    grep -Fx 'mode en' "$TANGO_IME_IBUS_TRACE"
    test "$(grep '^mode ' "$TANGO_IME_IBUS_TRACE" | tail -1)" = 'mode zh'
    /usr/bin/python3 "$here/smoke.py" | tee "$out/ibus-panel-selection.txt"
else
    if grep -Fq 'standalone left Shift is reserved by Fcitx5' "$daemon_log"; then exit 1; fi
    grep -Eq 'Tango Fcitx5 candidates [1-9][0-9]*$' "$out/fcitx-daemon.log"
    grep -F 'Tango Fcitx5 mode en' "$out/fcitx-daemon.log"
    test "$(grep 'Tango Fcitx5 mode ' "$out/fcitx-daemon.log" | tail -1)" = 'Tango Fcitx5 mode zh'
    # Switch away with an active preedit. The host should commit it to
    # the same GTK entry before the addon releases that context.
    xdotool key n i
    sleep 0.2
    fcitx5-remote -s keyboard-us
    sleep 0.2
    test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc你ni
    fcitx5-remote -s "$engine"
    sleep 0.2
    xdotool key n i
    sleep 0.2
    xdotool mousemove --window "$window" 375 75 click 1
    sleep 0.3
    test "$(cat "$focus_file")" = secondary
    test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc你nini
    xdotool mousemove --window "$window" 375 25 click 1
    sleep 0.2
    xdotool key End
    printf '%s\n' "Fcitx5 commits preedit once on GTK focus-out"
    host=$(/usr/bin/python3 - <<'PYTHON'
import os
import socket
import struct
from pathlib import Path

name = "com.tangoui.inputmethod.example_Connection.sock"
with socket.socket(socket.AF_UNIX) as client:
    client.connect(str(Path(os.environ["XDG_RUNTIME_DIR"]) / "tango-ime" / name))
    print(struct.unpack("3i", client.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12))[0])
PYTHON
)
    test "$(readlink "/proc/$host/exe")" = /usr/bin/TangoPinyin
    pids+=("$host")
    kill -STOP "$host"
    xdotool key z
    sleep 1.5
    xdotool key q
    sleep 0.2
    test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc你ninizq
    kill -KILL "$host"
    sleep 0.2
    xdotool key x
    sleep 0.2
    test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc你ninizqx
    echo "Fcitx5 preedit on switch, stopped-host pass-through and disconnected-host pass-through verified"
fi
echo "$framework keyboard smoke passed" | tee "$out/$framework-result.txt"
