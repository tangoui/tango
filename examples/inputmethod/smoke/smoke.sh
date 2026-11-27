#!/bin/bash
# Run in a macOS GUI login session. Requires cua-driver with existing permissions.
set -euo pipefail

example=$(cd -- "$(dirname -- "$0")/.." && pwd)
repo=$(cd -- "$example/../.." && pwd)
output=${1:?usage: smoke.sh OUTPUT_DIR}
mkdir -p -- "$output"
output=$(cd -- "$output" && pwd)
exec > >(tee "$output/smoke.log") 2>&1

source_id=com.tangoui.inputmethod.example
bundle="$HOME/Library/Input Methods/Tango Pinyin.app"
session=tango-ime-smoke
work=$(mktemp -d)
tis="$work/tis"
installed=0
restore=0
pid=
window=
focus_window=
original=

window_args() {
	jq -nc --argjson pid "$pid" --argjson window "$1" --arg session "$session" \
		'{pid:$pid, window_id:$window, session:$session}'
}

close_doc() {
	local args
	args=$(window_args "$1")
	cua-driver call run_script "$(jq -nc --argjson target "$args" --arg script "$close_script" \
		'{session:$target.session, script:("const target = " + ($target|tojson) + ";\n" + $script)}')" | jq -e '.ok' >/dev/null
}

close_script=$(cat <<'JS'
const {pid, window_id: windowId} = target;
if (!(await cua.listWindows({pid})).some(w => w.window_id === windowId)) return;
const app = await cua.getApp({pid, windowId});
await cua.call("press_key", {pid, window_id: windowId, key: "w", modifiers: ["cmd"], delivery_mode: "background"});
for (let i = 0; i < 15; i++) {
  if (!(await cua.listWindows({pid})).some(w => w.window_id === windowId)) return;
  const state = await app.getState({tree_format: "elements", include_screenshot: false});
  const discard = state.elements?.find(e => e.role === "AXButton" &&
    /^(Delete|Don.t Save|删除|不存储|不保存)$/.test(e.label ?? ""));
  // The close button is the first unlabeled AppKit window control.
  const close = state.elements?.find(e => e.role === "AXButton" && e.parent_index === 0 &&
    !e.label && e.actions?.includes("AXPress"));
  const button = discard ?? close;
  if (button) {
    try { await app.click(button.element_token); }
    catch (error) {
      // AppKit can report a stale element after successfully closing it.
      if ((await cua.listWindows({pid})).some(w => w.window_id === windowId)) throw error;
      return;
    }
  }
  await cua.sleep(100);
}
throw new Error("Smoke TextEdit document is still open");
JS
)

cleanup() {
	local code=$? current state
	trap - EXIT
	set +e
	if [[ "$restore" = 1 ]]; then
		current=$("$tis" current)
		if [[ "$current" != "$original" ]]; then
			"$tis" select "$original" || code=1
			current=$("$tis" current)
		fi
		echo "Restored input source: $current"
		[[ "$current" = "$original" ]] || code=1
	fi
	if [[ "$installed" = 1 ]]; then
		state=$("$tis" status "$source_id")
		if [[ "$state" = enabled ]]; then
			"$tis" disable "$source_id" || code=1
			echo "Disabled test input source"
		fi
		killall -q TangoPinyin || true
		rm -rf -- "$bundle"
		[[ ! -e "$bundle" ]] || code=1
		state=$("$tis" status "$source_id")
		echo "Test input source after removing bundle: $state"
		[[ "$state" = missing || "$state" = disabled ]] || code=1
	fi
	if [[ -n "$focus_window" ]]; then
		if close_doc "$focus_window"; then echo "Closed TextEdit focus test document"; else code=1; fi
	fi
	if [[ -n "$window" ]]; then
		if close_doc "$window"; then echo "Closed TextEdit smoke document"; else code=1; fi
	fi
	rm -rf -- "$work"
	exit "$code"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

cua-driver call check_permissions '{"prompt":false}' | jq -e '.accessibility and .screen_recording' >/dev/null || {
	echo "Cua Driver needs Accessibility and Screen Recording permissions in System Settings." >&2
	exit 1
}
(cd "$repo" && CGO_ENABLED=0 go build -o "$work/tan" ./cmd/tan && CGO_ENABLED=0 go build -o "$tis" ./examples/inputmethod/smoke)
(cd "$example" && "$work/tan" build)
app="$example/build/darwin-$(go env GOARCH)/Tango Pinyin.app"
plutil -lint -- "$app/Contents/Info.plist"

original=$("$tis" current)
echo "Original input source: $original"
"$tis" status "$source_id" | jq -Re '. != "enabled"' >/dev/null
if [[ -e "$bundle" || -L "$bundle" ]]; then
	echo "Already installed: $bundle" >&2
	exit 1
fi
mkdir -p -- "$(dirname -- "$bundle")"
installed=1
ditto "$app" "$bundle"
killall -q TangoPinyin || true
"$tis" register "$bundle"
"$tis" enable "$source_id"

first="$work/Tango IME Smoke.txt"
second="$work/Tango IME Focus.txt"
touch "$first" "$second"
jq -nc --arg session "$session" --arg first "file://${first// /%20}" --arg second "file://${second// /%20}" \
	'{bundle_id:"com.apple.TextEdit", creates_new_application_instance:true, session:$session, urls:[$second,$first]}' \
	| cua-driver call launch_app > "$output/textedit-launch.json"
pid=$(jq -er '.pid' "$output/textedit-launch.json")
for ((attempt=0; attempt<25; attempt++)); do
	windows=$(cua-driver call list_windows "$(jq -nc --argjson pid "$pid" --arg session "$session" '{pid:$pid,session:$session}')")
	window=$(jq -r '[.windows[] | select(.title | startswith("Tango IME Smoke"))][0].window_id // empty' <<< "$windows")
	focus_window=$(jq -r '[.windows[] | select(.title | startswith("Tango IME Focus"))][0].window_id // empty' <<< "$windows")
	if [[ -n "$window" && -n "$focus_window" ]]; then break; fi
	sleep 0.2
done
if [[ -z "$window" || -z "$focus_window" ]]; then
	echo "The two TextEdit smoke windows did not open." >&2
	exit 1
fi

restore=1
selected=
for ((attempt=0; attempt<20; attempt++)); do
	if "$tis" select "$source_id"; then
		selected=$("$tis" current)
		if [[ "$selected" = "$source_id" ]]; then break; fi
	fi
	sleep 0.2
done
echo "Selected input source: $selected"
if [[ "$selected" != "$source_id" ]]; then exit 1; fi

script=$(cat <<'JS'
const editor = await cua.getApp({pid: target.pid, windowId: target.window_id});
const other = await cua.getApp({pid: target.pid, windowId: target.focus_window});
async function document(app) {
  const state = await app.getState({tree_format: "elements", include_screenshot: false});
  const field = state.elements?.find(e => e.role === "AXTextArea");
  if (!field) throw new Error("TextEdit document field missing");
  return field;
}
async function keys(sequence, windowId = target.window_id, focusedField) {
  for (const key of sequence) {
    const result = await cua.call("press_key", {
      pid: target.pid, window_id: windowId, key, element_token: focusedField,
      delivery_mode: "background", post_action_wait: "none"
    });
    if (result.effect === "refused") throw new Error(JSON.stringify(result));
    focusedField = undefined;
  }
}
async function value() {
  return (await document(editor)).value ?? "";
}
await keys([..."nihao", "space", ..."shijie", "escape", ..."wo", "return"],
  target.window_id, (await document(editor)).element_token);
const committed = await value();
if (committed !== "你好wo") return {committed, expected: "你好wo"};
await keys(["n", "i"]);
const composing = await value();
await keys(["right"], target.focus_window, (await document(other)).element_token);
// After changing focus, Escape on returning to the first document must
// leave the letters in place if the composition ended.
await keys(["escape"], target.window_id, (await document(editor)).element_token);
return {committed, composing, afterFocusChange: await value()};
JS
)
jq -nc --argjson target "$(window_args "$window")" --argjson focus "$focus_window" --arg script "$script" \
	'{session:$target.session, script:("const target = " + ($target + {focus_window:$focus}|tojson) + ";\n" + $script)}' \
	| cua-driver call run_script > "$output/textedit-smoke.json"
jq -r '.value // .result // .error' "$output/textedit-smoke.json" | tee "$output/textedit-readback.txt"
if ! pgrep -x TangoPinyin; then echo "The Tango Pinyin host was not launched during background key delivery."; fi
cua-driver call --screenshot-out-file "$output/textedit-smoke.png" get_window_state \
	"$(jq -c '. + {include_accessibility_tree:false}' <<< "$(window_args "$window")")" > "$output/textedit-capture.json"
jq -e '(.value // .result) | .committed == "你好wo" and .afterFocusChange == "你好woni"' "$output/textedit-smoke.json" >/dev/null
test -s "$output/textedit-smoke.png"
