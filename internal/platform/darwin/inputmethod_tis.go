//go:build darwin && !ios

package darwin

import (
	"errors"
	"fmt"

	"github.com/ebitengine/purego/objc"
)

type imeInputSource struct {
	current      func() string
	selectMode   func(string) error
	notification uintptr
}

func loadIMEInputSource() (imeInputSource, error) {
	var l linker
	carbon := l.open(frameworksDir + "Carbon.framework/Carbon")
	if carbon == 0 {
		return imeInputSource{}, errors.Join(l.failures...)
	}
	var current func() id
	var list func(id, bool) id
	var getProperty func(id, uintptr) id
	var selectSource func(id) int32
	var cfRelease func(id)
	l.bind(&current, carbon, "TISCopyCurrentKeyboardInputSource")
	l.bind(&list, carbon, "TISCreateInputSourceList")
	l.bind(&getProperty, carbon, "TISGetInputSourceProperty")
	l.bind(&selectSource, carbon, "TISSelectInputSource")
	l.bind(&cfRelease, libCF, "CFRelease")
	if err := errors.Join(l.failures...); err != nil {
		return imeInputSource{}, err
	}
	key := constPtr(carbon, "kTISPropertyInputSourceID")
	if key == 0 {
		return imeInputSource{}, errors.New("TIS input source ID key unavailable")
	}
	return imeInputSource{
		notification: constPtr(carbon, "kTISNotifySelectedKeyboardInputSourceChanged"),
		current: func() string {
			source := current()
			if source == 0 {
				return ""
			}
			defer cfRelease(source)
			return goString(getProperty(source, key))
		},
		selectMode: func(sourceID string) error {
			filter := send(class("NSDictionary"), "dictionaryWithObject:forKey:", uintptr(nsString(sourceID)), key)
			sources := list(filter, false)
			if sources == 0 {
				return fmt.Errorf("input source %q is unavailable", sourceID)
			}
			defer cfRelease(sources)
			if send(sources, "count") == 0 {
				return fmt.Errorf("input source %q is disabled", sourceID)
			}
			if status := selectSource(send(sources, "objectAtIndex:", 0)); status != 0 {
				return fmt.Errorf("select input source %q: OSStatus %d", sourceID, status)
			}
			return nil
		},
	}, nil
}

func watchIMEInputSource(name uintptr) {
	if name == 0 {
		return
	}
	classDef("TangoIMEInputSourceObserver", "NSObject", nil, []objc.MethodDef{
		method("inputSourceChanged:", func(_ id, _ objc.SEL, _ id) {
			runOnMain(imeObserveInputSource)
		}),
	})
	observer := alloc("TangoIMEInputSourceObserver") // lives as long as the IMK server
	center := send(class("NSDistributedNotificationCenter"), "defaultCenter")
	send(center, "addObserver:selector:name:object:", uintptr(observer),
		uintptr(sel("inputSourceChanged:")), name, 0)
}
