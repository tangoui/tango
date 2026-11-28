#!/usr/bin/env bash
set -Eeuo pipefail
trap 'printf "Smoke failure at line %s: %s\n" "$LINENO" "$BASH_COMMAND" >&2' ERR

framework=${1:?ibus or fcitx}
out=${2:?evidence directory}
here=$(cd "$(dirname "$0")" && pwd)
engine=com.tangoui.inputmethod.example
session=$(mktemp -d)
pids=()
cleanup() {
    for pid in "${pids[@]}"; do kill "$pid" 2>/dev/null || true; done
    wait 2>/dev/null || true
    rm -rf -- "$session"
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
        if ibus list-engine 2>/dev/null | grep -Fq "$engine"; then break; fi
        sleep 0.2
    done
    ibus engine "$engine"
else
    mkdir -p "$XDG_CONFIG_HOME/fcitx5"
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
    fcitx5-remote -s "$engine"
fi

export TANGO_SMOKE_TEXT="$out/$framework-text.txt"
: > "$TANGO_SMOKE_TEXT"
/usr/bin/python3 "$here/gtk-smoke.py" >"$out/$framework-client.log" 2>&1 &
pids+=("$!")
window=$(timeout 10 xdotool search --sync --onlyvisible --name '^Tango IME Smoke$')
xdotool windowfocus --sync "$window"
if [[ "$framework" == ibus ]]; then
    ibus engine "$engine"
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
    pkill -KILL -u "$(id -u)" -x TangoPinyin
    sleep 0.2
    xdotool key z
    sleep 0.2
    test "$(cat "$TANGO_SMOKE_TEXT")" = 你好abc你z
fi
echo "$framework keyboard smoke passed" | tee "$out/$framework-result.txt"
