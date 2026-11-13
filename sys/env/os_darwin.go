//go:build darwin

package env

import "syscall"

func osVersion() (name, version string) {
	v, _ := syscall.Sysctl("kern.osproductversion")
	name = "macOS"
	if isIOS {
		name = "iOS"
	}
	return name, v
}
