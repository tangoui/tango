//go:build windows

package autostart

import (
	"errors"
	"syscall"
	"unsafe"
)

const (
	runKey             = `Software\Microsoft\Windows\CurrentVersion\Run`
	startupApprovedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`

	hkeyCurrentUser = 0x80000001
	keyQueryValue   = 0x0001
	keySetValue     = 0x0002
	regSZ           = 1
	errFileNotFound = 2
)

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegOpenKeyExW    = advapi32.NewProc("RegOpenKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegQueryValueExW = advapi32.NewProc("RegQueryValueExW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	procRegCloseKey      = advapi32.NewProc("RegCloseKey")
)

func regErr(what string, r uintptr) error {
	return errors.New("tango/autostart: " + what + ": " + syscall.Errno(r).Error())
}

func keyPath(o Options) string {
	if o.Dir != "" {
		return o.Dir
	}
	return runKey
}

func openKey(path string, access uint32, create bool) (uintptr, uintptr) {
	p, _ := syscall.UTF16PtrFromString(path)
	var key uintptr
	var r uintptr
	if create {
		r, _, _ = procRegCreateKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(p)), 0, 0, 0,
			uintptr(access), 0, uintptr(unsafe.Pointer(&key)), 0)
	} else {
		r, _, _ = procRegOpenKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(p)), 0, uintptr(access), uintptr(unsafe.Pointer(&key)))
	}
	return key, r
}

func enable(o Options, cmd []string) error {
	key, r := openKey(keyPath(o), keySetValue, true)
	if r != 0 {
		return regErr("RegCreateKeyExW", r)
	}
	defer procRegCloseKey.Call(key)
	name, _ := syscall.UTF16PtrFromString(o.ID)
	val, err := syscall.UTF16FromString(windowsCommandLine(cmd))
	if err != nil {
		return err
	}
	r, _, _ = procRegSetValueExW.Call(key, uintptr(unsafe.Pointer(name)), 0, regSZ,
		uintptr(unsafe.Pointer(&val[0])), uintptr(len(val)*2))
	if r != 0 {
		return regErr("RegSetValueExW", r)
	}
	return nil
}

func disable(o Options) error {
	key, r := openKey(keyPath(o), keySetValue, false)
	if r == errFileNotFound {
		return nil
	}
	if r != 0 {
		return regErr("RegOpenKeyExW", r)
	}
	defer procRegCloseKey.Call(key)
	name, _ := syscall.UTF16PtrFromString(o.ID)
	if r, _, _ := procRegDeleteValueW.Call(key, uintptr(unsafe.Pointer(name))); r != 0 && r != errFileNotFound {
		return regErr("RegDeleteValueW", r)
	}
	return nil
}

// queryValue returns the raw data of a value; found is false when the key
// or value does not exist.
func queryValue(path, value string) (data []byte, found bool, err error) {
	key, r := openKey(path, keyQueryValue, false)
	if r == errFileNotFound {
		return nil, false, nil
	}
	if r != 0 {
		return nil, false, regErr("RegOpenKeyExW", r)
	}
	defer procRegCloseKey.Call(key)
	name, _ := syscall.UTF16PtrFromString(value)
	var size uint32
	r, _, _ = procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(name)), 0, 0, 0, uintptr(unsafe.Pointer(&size)))
	if r == errFileNotFound {
		return nil, false, nil
	}
	if r != 0 {
		return nil, false, regErr("RegQueryValueExW", r)
	}
	if size == 0 {
		return nil, true, nil
	}
	buf := make([]byte, size)
	r, _, _ = procRegQueryValueExW.Call(key, uintptr(unsafe.Pointer(name)), 0, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return nil, false, regErr("RegQueryValueExW", r)
	}
	return buf[:size], true, nil
}

func isEnabled(o Options) (bool, error) {
	_, found, err := queryValue(keyPath(o), o.ID)
	if err != nil || !found {
		return false, err
	}
	if o.Dir != "" {
		return true, nil // a custom key has no Task Manager state
	}
	approved, _, err := queryValue(startupApprovedKey, o.ID)
	if err != nil {
		return true, nil
	}
	return startupApprovedEnabled(approved), nil
}
