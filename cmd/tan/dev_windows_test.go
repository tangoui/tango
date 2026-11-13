package main

import (
	"strconv"
	"syscall"
	"testing"
	"time"
)

// fakeDevServer is a dev server command line for cmd.exe: a grandchild that
// would outlive a plain kill of the shell (its pid in child.pid), then the
// URL line, then wait.
const fakeDevServer = `powershell -NoProfile -Command "$p = Start-Process -PassThru -WindowStyle Hidden ping -ArgumentList '-n','60','127.0.0.1'; Set-Content -Encoding ascii child.pid $p.Id; Write-Output '  Local: http://localhost:4321/'; Wait-Process -Id $p.Id"`

// assertGone fails unless the process pid has exited.
func assertGone(t *testing.T, pid string) {
	t.Helper()
	n, err := strconv.Atoi(pid)
	if err != nil {
		t.Fatal(err)
	}
	const processQueryLimitedInformation = 0x1000
	const stillActive = 259
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(n))
		if err != nil {
			return
		}
		var code uint32
		err = syscall.GetExitCodeProcess(h, &code)
		syscall.CloseHandle(h)
		if err != nil || code != stillActive {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Errorf("child %d of the dev server survived stop", n)
}
