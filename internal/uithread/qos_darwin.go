//go:build darwin

package uithread

import (
	"sync"

	"github.com/ebitengine/purego"
)

// qosClassUserInteractive is QOS_CLASS_USER_INTERACTIVE (sys/qos.h), the
// class macOS gives the main thread.
const qosClassUserInteractive = 0x21

var (
	qosOnce    sync.Once
	setQoSSelf func(qos uint32, relPriority int32) int32
)

// raiseQoS gives the calling (locked) thread the main thread's
// user-interactive QoS: a window UI thread draws and presents against the
// display's deadline, and a Go-created thread's default class loses the
// CPU to it under load. Best effort: a missing symbol leaves the default.
func raiseQoS() {
	qosOnce.Do(func() {
		lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		if _, err := purego.Dlsym(lib, "pthread_set_qos_class_self_np"); err != nil {
			return
		}
		purego.RegisterLibFunc(&setQoSSelf, lib, "pthread_set_qos_class_self_np")
	})
	if setQoSSelf != nil {
		setQoSSelf(qosClassUserInteractive, 0)
	}
}
