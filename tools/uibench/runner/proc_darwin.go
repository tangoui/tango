package main

import (
	"errors"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// libproc and mach, through purego so the runner builds without cgo.
var (
	procPidRusage    func(pid int32, flavor int32, buf unsafe.Pointer) int32
	machTimebaseInfo func(info unsafe.Pointer) int32

	procOnce sync.Once
	procErr  error
	tbNumer  uint64
	tbDenom  uint64
)

func procInit() {
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		procErr = err
		return
	}
	purego.RegisterLibFunc(&procPidRusage, lib, "proc_pid_rusage")
	purego.RegisterLibFunc(&machTimebaseInfo, lib, "mach_timebase_info")
	var tb [2]uint32 // mach_timebase_info_data_t: numer, denom
	if machTimebaseInfo(unsafe.Pointer(&tb)) != 0 || tb[1] == 0 {
		procErr = errors.New("mach_timebase_info failed")
		return
	}
	tbNumer, tbDenom = uint64(tb[0]), uint64(tb[1])
}

// rusageV2 is struct rusage_info_v2 (RUSAGE_INFO_V2 = 2) as uint64 words:
// [0:2] the UUID, [2] ri_user_time, [3] ri_system_time, …,
// [9] ri_phys_footprint, followed by fields the runner does not read.
type rusageV2 [32]uint64

func rusage(pid int) (rusageV2, error) {
	procOnce.Do(procInit)
	var u rusageV2
	if procErr != nil {
		return u, procErr
	}
	if procPidRusage(int32(pid), 2, unsafe.Pointer(&u)) != 0 {
		return u, errors.New("proc_pid_rusage failed")
	}
	return u, nil
}

// procCPU is the process' user+system time. The kernel counts it in Mach
// absolute time units, which mach_timebase_info converts to nanoseconds.
func procCPU(pid int) (time.Duration, error) {
	u, err := rusage(pid)
	if err != nil {
		return 0, err
	}
	return time.Duration((u[2] + u[3]) * tbNumer / tbDenom), nil
}

// procFootprint is the process' physical footprint, Activity Monitor's
// "Memory".
func procFootprint(pid int) (int64, error) {
	u, err := rusage(pid)
	if err != nil {
		return 0, err
	}
	return int64(u[9]), nil
}
