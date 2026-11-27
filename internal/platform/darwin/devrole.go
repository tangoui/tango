//go:build darwin && !ios

package darwin

// devWorker (devrole_dev.go, devrole_prod.go) is set in a `tan dev`
// hot-reload worker. Its windows live in the dev host, so it starts as an
// accessory app: no Dock icon, and it does not take activation from the
// editor or the host when it launches.

// activationPolicy is the policy the app starts with.
func activationPolicy() uintptr {
	bundle := send(class("NSBundle"), "mainBundle")
	info := send(bundle, "infoDictionary")
	background := send(info, "objectForKey:", uintptr(nsString("LSBackgroundOnly")))
	uiElement := send(info, "objectForKey:", uintptr(nsString("LSUIElement")))
	return policyFor(devWorker, plistBool(background), plistBool(uiElement))
}

// Both NSNumber and NSString implement boolValue.
func plistBool(value id) bool {
	return respondsTo(value, "boolValue") && sendBool(value, "boolValue")
}

func policyFor(worker, backgroundOnly, uiElement bool) uintptr {
	const nsApplicationActivationPolicyProhibited = 2
	switch {
	case backgroundOnly:
		return nsApplicationActivationPolicyProhibited
	case worker || uiElement:
		return nsApplicationActivationPolicyAccessory
	default:
		return nsApplicationActivationPolicyRegular
	}
}
