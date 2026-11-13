//go:build darwin && !ios

package darwin

// devWorker (devrole_dev.go, devrole_prod.go) is set in a `tan dev`
// hot-reload worker. Its windows live in the dev host, so it starts as an
// accessory app: no Dock icon, and it does not take activation from the
// editor or the host when it launches.

// activationPolicy is the policy the app starts with.
func activationPolicy() uintptr {
	if devWorker {
		return nsApplicationActivationPolicyAccessory
	}
	return nsApplicationActivationPolicyRegular
}
