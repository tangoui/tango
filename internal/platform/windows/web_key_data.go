package windows

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

// Chromium's Input.dispatchKeyEvent modifier bits.
const (
	webAlt = 1 << iota
	webCtrl
	webMeta
	webShift
)

var webVirtualKeys = func() map[input.Key]uint16 {
	codes := make(map[input.Key]uint16)
	for vk, k := range canvasVKKeys {
		codes[k] = vk
	}
	for k := input.KeyMinus; k <= input.KeyBackquote; k++ {
		codes[k] = namedKeys[string(k.Rune())].vk
	}
	for k := input.KeyNumpad0; k <= input.KeyNumpad9; k++ {
		codes[k] = canvasVKNumpad0 + uint16(k-input.KeyNumpad0)
	}
	codes[input.KeyNumpadEnter] = canvasVKReturn
	for k := input.KeyF1; k <= input.KeyF12; k++ {
		codes[k] = canvasVKF1 + uint16(k-input.KeyF1)
	}
	return codes
}()

type webKeyPlan struct {
	vk                              uint16
	key, code, text, unmodifiedText string
	mods, releasedMods, location    int
	accelMods                       modifiers
}

func planWebKey(key input.Key, mods input.Modifiers) (webKeyPlan, bool) {
	p := webKeyPlan{vk: webVirtualKeys[key], code: key.String()}
	r := key.Rune()
	switch {
	case r >= 'a' && r <= 'z':
		p.vk = uint16(r - 'a' + 'A')
		p.code = "Key" + key.String()
	case key >= input.Key0 && key <= input.Key9:
		p.vk = uint16(r)
		p.code = "Digit" + key.String()
	}
	if p.vk == 0 {
		return webKeyPlan{}, false
	}
	for _, m := range [...]struct {
		input input.Modifiers
		cdp   int
		accel modifiers
	}{{input.ModAlt, webAlt, modAlt}, {input.ModCtrl, webCtrl, modCtrl}, {input.ModMeta, webMeta, 0}, {input.ModShift, webShift, modShift}} {
		if mods&m.input != 0 {
			p.mods |= m.cdp
			p.accelMods |= m.accel
		}
	}
	p.releasedMods = p.mods
	p.key = p.code
	switch {
	case key >= input.KeyNumpad0 && key <= input.KeyNumpadEnter:
		p.location = 3
	case key >= input.KeyShiftLeft && key <= input.KeyMetaRight:
		names := [...]string{"Shift", "Control", "Alt", "Meta"}
		bits := [...]int{webShift, webCtrl, webAlt, webMeta}
		index := key - input.KeyShiftLeft
		p.key, p.location = names[index/2], 1+int(index%2)
		p.code = p.key + [...]string{"Left", "Right"}[index%2]
		p.mods |= bits[index/2]
		p.releasedMods &^= bits[index/2]
	}
	switch {
	case r != 0:
		p.key = string(platform.WebKeyRune(key, mods))
		if p.mods&(webCtrl|webAlt|webMeta) == 0 {
			p.text, p.unmodifiedText = p.key, p.key
		}
	case key >= input.KeyLeft && key <= input.KeyDown:
		p.key = "Arrow" + p.code
		p.code = p.key
	case key == input.KeyNumpadEnter:
		p.key = "Enter"
	}
	if p.key == "Enter" && p.mods&(webCtrl|webAlt|webMeta) == 0 {
		p.text, p.unmodifiedText = "\r", "\r"
	}
	return p, true
}

// menu returns the same command WebView2's AcceleratorKeyPressed would
// choose. Editing accelerators are handled in Chromium, with an explicit
// editing command attached to the synthetic key.
func (p webKeyPlan) menu(plan *menuPlan) (cmd uint16, edit string) {
	if plan == nil || p.mods&webMeta != 0 {
		return 0, ""
	}
	if cmd, ok := matchAccel(plan.Accels, p.vk, p.accelMods); ok {
		return cmd, ""
	}
	if cmd, ok := matchAccel(plan.EditAccels, p.vk, p.accelMods); ok {
		edit, _ := roleEditCommand(plan.Actions[cmd].Role)
		return 0, edit
	}
	return 0, ""
}

func (p webKeyPlan) event(down bool, edit string) map[string]any {
	args := map[string]any{
		"type": "keyUp", "key": p.key, "code": p.code, "modifiers": p.releasedMods,
		"windowsVirtualKeyCode": p.vk, "nativeVirtualKeyCode": p.vk,
	}
	if p.location == 3 {
		args["isKeypad"] = true
	} else if p.location != 0 {
		args["location"] = p.location
	}
	if down {
		args["type"], args["modifiers"] = "rawKeyDown", p.mods
		if p.text != "" {
			args["type"], args["text"], args["unmodifiedText"] = "keyDown", p.text, p.unmodifiedText
		}
		if edit != "" {
			args["commands"] = []string{edit}
		}
	}
	return args
}
