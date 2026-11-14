//go:build darwin && cgo

package main

/*
#include <mach/mach.h>
#include <mach/task_info.h>

// footprint is the process's phys_footprint (what Xcode's memory gauge and
// jetsam use), or 0.
static uint64_t footprint(void) {
	task_vm_info_data_t info;
	mach_msg_type_number_t n = TASK_VM_INFO_COUNT;
	if (task_info(mach_task_self(), TASK_VM_INFO, (task_info_t)&info, &n) != KERN_SUCCESS) return 0;
	return info.phys_footprint;
}
*/
import "C"

// footprint reports the app process's physical memory footprint in bytes
// (0 if unknown). The self-test compares it before and after streaming
// tens of MiB through the tango:// scheme handler.
func footprint() uint64 { return uint64(C.footprint()) }
