#!/usr/bin/env bash
set -Eeuo pipefail
trap 'printf "Smoke failure at line %s: %s\n" "$LINENO" "$BASH_COMMAND" >&2' ERR

framework=${1:?ibus or fcitx}
out=${2:?evidence directory}
here=$(cd "$(dirname "$0")" && pwd)
engine=com.tangoui.inputmethod.example
expect=${3:-installed}
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
Xvfb -displayfd 3 -screen 0 800x600x24 -nolisten tcp 3>"$session/display" >"$out/$framework-xvfb.log" 2>&1 &
pids+=("$!")
for i in {1..50}; do
    [[ -s "$session/display" ]] && break
    sleep 0.1
done
export DISPLAY=":$(cat "$session/display")"
export GDK_BACKEND=x11 GTK_IM_MODULE="$framework" XMODIFIERS="@im=$framework"
dbus-update-activation-environment DISPLAY GTK_IM_MODULE XMODIFIERS XDG_CONFIG_HOME XDG_DATA_HOME XDG_RUNTIME_DIR TANGO_IME_SMOKE_TRACE

if [[ "$framework" == ibus ]]; then
    export TANGO_IME_IBUS_TRACE="$out/ibus-panel.log"
    ibus-daemon --xim --panel=disable --config=disable >"$out/ibus-daemon.log" 2>&1 &
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
    # Reserve a standalone Shift for Tango. Fcitx defaults to using it to
    # temporarily switch between the current engine and the first engine.
    cat >"$XDG_CONFIG_HOME/fcitx5/config" <<'CONFIG'
[Hotkey/AltTriggerKeys]
0=Control+Shift_L
CONFIG
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
    fcitx5 --replace >"$out/fcitx-daemon.log" 2>&1 &
    pids+=("$!")
    for i in {1..50}; do
        if fcitx5-remote >/dev/null 2>&1; then break; fi
        sleep 0.2
    done
    if [[ "$expect" == removed ]]; then
        gdbus call --session --dest org.fcitx.Fcitx5 --object-path /controller \
            --method org.fcitx.Fcitx.Controller1.AvailableInputMethods >"$out/fcitx-after-removal.txt"
        if grep -Fq "$engine" "$out/fcitx-after-removal.txt"; then exit 1; fi
        exit 0
    fi
    fcitx5-remote -s "$engine"
    if [[ "$expect" == panel-preedit ]]; then
        /usr/bin/python3 "$here/fcitx-panel-smoke.py" | tee "$out/fcitx-panel-focus.txt"
        exit 0
    fi
fi

export TANGO_SMOKE_TEXT="$out/$framework-text.txt"
: > "$TANGO_SMOKE_TEXT"
/usr/bin/python3 "$here/gtk-smoke.py" >"$out/$framework-client.log" 2>&1 &
pids+=("$!")
window=$(timeout 10 xdotool search --sync --onlyvisible --name '^Tango IME Smoke$')
xdotool windowfocus --sync "$window"
if [[ "$framework" == ibus ]]; then
    for i in {1..25}; do
        if ibus engine "$engine" && [[ "$(ibus engine 2>/dev/null)" == "$engine" ]]; then break; fi
        sleep 0.2
    done
    test "$(ibus engine)" = "$engine"
else
    fcitx5-remote -o
    fcitx5-remote -s "$engine"
fi
sleep 1
for key in n i h a o space; do
    xdotool key "$key"
    sleep 0.1
done
sleep 0.3
test "$(cat "$TANGO_SMOKE_TEXT")" = 你好
xdotool key Shift_L
sleep 0.2
for key in a b c; do xdotool key "$key"; done
sleep 0.2
test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc
if [[ "$framework" == fcitx ]]; then
    test "$(fcitx5-remote)" = 2
    test "$(fcitx5-remote -n)" = "$engine"
fi
xdotool key Shift_L
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
else
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
    test "$(cat "${TANGO_SMOKE_TEXT%.txt}.focus")" = secondary
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
