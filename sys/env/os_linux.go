//go:build linux

package env

import (
	"os"
	"strings"
	"syscall"
)

func osVersion() (name, version string) {
	var u syscall.Utsname
	if syscall.Uname(&u) == nil {
		version = utsString(u.Release[:])
	}
	name = "Linux"
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		if n := osReleaseName(string(data)); n != "" {
			name = n
		}
	} else if _, err := os.Stat("/system/build.prop"); err == nil {
		name = "Android"
	}
	return name, version
}

func utsString[T int8 | uint8](f []T) string {
	var b strings.Builder
	for _, c := range f {
		if c == 0 {
			break
		}
		b.WriteByte(byte(c))
	}
	return b.String()
}
