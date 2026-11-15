//go:build windows

package single

import (
	"errors"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

var (
	procGetSecurityInfo        = advapi32.NewProc("GetSecurityInfo")
	procConvertSDToStringSD    = advapi32.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
	seKernelObject             = uintptr(6) // SE_KERNEL_OBJECT
	daclSecurityInformation    = uintptr(4) // DACL_SECURITY_INFORMATION
	sddlRevision1ForConversion = uintptr(1)
)

// daclOf returns the DACL of handle h as SDDL.
func daclOf(t *testing.T, h syscall.Handle) string {
	t.Helper()
	var sd uintptr
	r, _, _ := procGetSecurityInfo.Call(uintptr(h), seKernelObject, daclSecurityInformation, 0, 0, 0, 0, uintptr(unsafe.Pointer(&sd)))
	if r != 0 {
		t.Fatalf("GetSecurityInfo: %v", syscall.Errno(r))
	}
	defer syscall.LocalFree(syscall.Handle(sd))
	var s *uint16
	if r, _, e := procConvertSDToStringSD.Call(sd, sddlRevision1ForConversion, daclSecurityInformation, uintptr(unsafe.Pointer(&s)), 0); r == 0 {
		t.Fatalf("ConvertSecurityDescriptorToStringSecurityDescriptor: %v", e)
	}
	defer syscall.LocalFree(syscall.Handle(unsafe.Pointer(s)))
	return syscall.UTF16ToString(unsafe.Slice(s, 1<<16))
}

func TestPipeNameHasSID(t *testing.T) {
	sid, err := currentSID()
	if err != nil || !strings.HasPrefix(sid, "S-1-") {
		t.Fatalf("SID %q %v", sid, err)
	}
	p, err := pipePath(name("k"))
	if err != nil || !strings.HasSuffix(p, "-"+sid) || !strings.HasPrefix(p, `\\.\pipe\tango-`) {
		t.Fatalf("pipe path %q %v", p, err)
	}
	if userTag() != sid {
		t.Fatalf("userTag %q", userTag())
	}
}

// The pipe's DACL is protected and has one entry: the current user.
func TestPipeDACL(t *testing.T) {
	sid, _ := currentSID()
	p, _ := pipePath(name(uniqueKey(t)))
	h, err := createPipe(p, true)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(h)
	c, err := openPipe(p) // GENERIC_READ includes READ_CONTROL
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(c)
	dacl := daclOf(t, c)
	// GA is reported mapped to the pipe's generic-all rights (FA).
	// SDDL writes well-known accounts as aliases ("LA" for the built-in
	// Administrator, which CI runs as); resolve the trustee to its SID.
	trustee := ""
	if i := strings.LastIndex(dacl, ";;;"); i >= 0 && strings.HasSuffix(dacl, ")") {
		trustee = dacl[i+3 : len(dacl)-1]
		if !strings.HasPrefix(trustee, "S-1-") {
			if s, err := syscall.StringToSid(trustee); err == nil {
				trustee, _ = s.String()
			}
		}
	}
	if !strings.HasPrefix(dacl, "D:P(") || strings.Count(dacl, "(") != 1 || trustee != sid {
		t.Fatalf("DACL %q, want only an allow entry for %s", dacl, sid)
	}
	// A second first instance fails while ours exists.
	if _, err := createPipe(p, true); err != errBusy {
		t.Fatalf("second first instance: %v", err)
	}
}

// The pipe server's owner is checked against the expected SID.
func TestCheckServer(t *testing.T) {
	sid, _ := currentSID()
	n := name(uniqueKey(t))
	srv, err := listen(n, func(message) {})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	p, _ := pipePath(n)
	h, err := openPipe(p)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(h)
	if err := checkServer(h, sid); err != nil {
		t.Fatalf("own server: %v", err)
	}
	if err := checkServer(h, "S-1-5-18"); !errors.Is(err, errUntrusted) {
		t.Fatalf("server checked against SYSTEM: %v, want errUntrusted", err)
	}
}

// A name held by a pipe that does not admit us (as another user's would
// not) is not taken for our primary: claim neither forwards nor waits for
// it, and Claim then runs the app on its own.
func TestClaimForeignPipe(t *testing.T) {
	n := name(uniqueKey(t))
	p, _ := pipePath(n)
	ps, _ := syscall.UTF16PtrFromString(p)
	sa, free, err := securityAttributes("D:P") // empty DACL: no access for anyone
	if err != nil {
		t.Fatal(err)
	}
	defer free()
	r, _, e := procCreateNamedPipeW.Call(uintptr(unsafe.Pointer(ps)), pipeAccessDuplex|fileFlagFirstPipeInstance,
		pipeRejectRemoteClients, pipeUnlimitedInstances, 4096, 4096, 0, uintptr(unsafe.Pointer(sa)))
	if syscall.Handle(r) == syscall.InvalidHandle {
		t.Fatal(e)
	}
	defer syscall.CloseHandle(syscall.Handle(r))
	srv, fwd, err := claim(n, []string{"secret"}, func(message) {}, timeout)
	if srv != nil || fwd || !errors.Is(err, errUntrusted) || errors.Is(err, errReach) {
		t.Fatalf("claim: srv=%v fwd=%v err=%v", srv, fwd, err)
	}
}
