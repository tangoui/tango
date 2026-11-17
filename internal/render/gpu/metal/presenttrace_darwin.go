//go:build darwin

package metal

import (
	"encoding/json"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/purego/objc"
	"golang.org/x/sys/unix"
)

var presentationOutput struct {
	once  sync.Once
	queue chan presentationRecord
	next  atomic.Uint64
}

// One process-lifetime, bounded diagnostics writer. No writer is spawned for
// each window, no IO is done on a native callback, and no Device is captured.
func nativePresentationTrace() *presentationTrace {
	if v := os.Getenv("TANGO_PRESENT_TRACE"); v == "" || v == "0" {
		return nil
	}
	presentationOutput.once.Do(func() {
		presentationOutput.queue = make(chan presentationRecord, 256)
		go func() {
			enc := json.NewEncoder(os.Stderr)
			for r := range presentationOutput.queue {
				_ = enc.Encode(struct {
					Type string `json:"type"`
					presentationRecord
				}{"metal-present", r})
			}
		}()
	})
	return newPresentationTrace(presentationOutput.next.Add(1), presentationOutput.queue)
}

func presentHostNS() int64 {
	var ts unix.Timespec
	if unix.ClockGettime(unix.CLOCK_UPTIME_RAW, &ts) != nil {
		return 0
	}
	return ts.Nano()
}

// watchDrawable must run before presentDrawable is encoded. The native API
// copies the block. Release our reference immediately after registration;
// the block's closure holds only a recorder and an immutable identity token.
func (t *presentationTrace) watchDrawable(drawable id, frame uint64) {
	if t == nil {
		return
	}
	r := t.begin(frame, msgU64(drawable, sel("drawableID")), presentHostNS())
	if !msgBool1(drawable, sel("respondsToSelector:"), sel("addPresentedHandler:")) {
		r.Status = "unsupported"
		t.emit(r)
		return
	}
	block := objc.NewBlock(func(_ objc.Block, shown id) {
		seconds := msgF64(shown, sel("presentedTime"))
		t.finish(r, int64(seconds*float64(time.Second)), presentHostNS())
	})
	msgSetID(drawable, sel("addPresentedHandler:"), id(block))
	block.Release()
}
