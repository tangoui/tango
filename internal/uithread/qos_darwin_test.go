//go:build darwin

package uithread

import (
	"testing"

	"github.com/ebitengine/purego"
)

// A window UI thread draws and presents on the display's deadline, as the
// main thread does; macOS runs the main thread at user-interactive QoS. A
// Go-created thread starts unspecified/default and loses the CPU to it
// under load (Task 11 run 2: more 20–32 ms intervals on the UI thread).
func TestThreadRunsAtUserInteractiveQoS(t *testing.T) {
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip(err)
	}
	var self func() uintptr
	var get func(thread uintptr, qos *uint32, rel *int32) int32
	purego.RegisterLibFunc(&self, lib, "pthread_self")
	purego.RegisterLibFunc(&get, lib, "pthread_get_qos_class_np")
	// Threads the Go runtime creates from the main thread inherit its
	// class; later ones (most, in a running app) start at default. Start
	// several so the test does not depend on which OS thread a Start gets.
	for i := range 8 {
		th := Start("qos")
		var qos uint32
		var rel int32
		var rc int32
		th.Do(func() { rc = get(self(), &qos, &rel) })
		th.Stop()
		if rc != 0 {
			t.Fatalf("pthread_get_qos_class_np: %d", rc)
		}
		if qos != qosClassUserInteractive {
			t.Fatalf("UI thread %d QoS class %#x, want user-interactive %#x", i, qos, qosClassUserInteractive)
		}
	}
}
