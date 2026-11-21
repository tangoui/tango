//go:build darwin && !ios

package darwin

import (
	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/platform"
)

func (b *backend) installActivityMonitor() {
	// Mouse buttons/motion/drag, keys/modifiers, scroll and trackpad gestures.
	const mask = 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<6 | 1<<7 |
		1<<10 | 1<<11 | 1<<12 | 1<<18 | 1<<19 | 1<<20 | 1<<22 |
		1<<25 | 1<<26 | 1<<27 | 1<<29 | 1<<30 | 1<<31 | 1<<32 | 1<<34
	block := objc.NewBlock(func(_ objc.Block, event objc.ID) objc.ID {
		ev := id(event)
		win := send(ev, "window")
		if win == 0 {
			switch send(ev, "type") {
			case 10, 11, 12:
				win = send(b.app, "keyWindow")
			}
		}
		if win != 0 {
			if s := b.byDelegate[send(win, "delegate")]; s != nil && !s.closed {
				platform.ReportActivity(b.h, s.id)
			}
		}
		return event
	})
	send(class("NSEvent"), "addLocalMonitorForEventsMatchingMask:handler:", mask, uintptr(block))
	block.Release() // AppKit keeps its copy.
}
