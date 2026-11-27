//go:build darwin && !ios

// tis uses Carbon Text Input Source Services to manage the smoke input source.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/ebitengine/purego"
)

type tis struct {
	createList  func(uintptr, bool) uintptr
	current     func() uintptr
	property    func(uintptr, uintptr) uintptr
	register    func(uintptr) int32
	enable      func(uintptr) int32
	selectInput func(uintptr) int32
	disable     func(uintptr) int32
	count       func(uintptr) int
	at          func(uintptr, int) uintptr
	string      func(uintptr, []byte, int, uint32) bool
	fileURL     func(uintptr, []byte, int, bool) uintptr
	release     func(uintptr)
	idKey       uintptr
}

func loadTIS() (t tis, err error) {
	carbon, err := purego.Dlopen("/System/Library/Frameworks/Carbon.framework/Carbon", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return t, err
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return t, err
	}
	purego.RegisterLibFunc(&t.createList, carbon, "TISCreateInputSourceList")
	purego.RegisterLibFunc(&t.current, carbon, "TISCopyCurrentKeyboardInputSource")
	purego.RegisterLibFunc(&t.property, carbon, "TISGetInputSourceProperty")
	purego.RegisterLibFunc(&t.register, carbon, "TISRegisterInputSource")
	purego.RegisterLibFunc(&t.enable, carbon, "TISEnableInputSource")
	purego.RegisterLibFunc(&t.selectInput, carbon, "TISSelectInputSource")
	purego.RegisterLibFunc(&t.disable, carbon, "TISDisableInputSource")
	purego.RegisterLibFunc(&t.count, cf, "CFArrayGetCount")
	purego.RegisterLibFunc(&t.at, cf, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&t.string, cf, "CFStringGetCString")
	purego.RegisterLibFunc(&t.fileURL, cf, "CFURLCreateFromFileSystemRepresentation")
	purego.RegisterLibFunc(&t.release, cf, "CFRelease")
	// These exported data symbols contain the addresses of the CFStrings.
	t.idKey, err = dataSymbol(carbon, "kTISPropertyInputSourceID")
	return t, err
}

func dataSymbol(library uintptr, name string) (uintptr, error) {
	addr, err := purego.Dlsym(library, name)
	if err != nil {
		return 0, err
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return *(*uintptr)(p), nil
}

func (t tis) sourceID(source uintptr) string {
	const utf8Encoding = 0x08000100
	value := t.property(source, t.idKey)
	var buf [1024]byte
	if value == 0 || !t.string(value, buf[:], len(buf), utf8Encoding) {
		return ""
	}
	return string(buf[:bytes.IndexByte(buf[:], 0)])
}

func (t tis) status(id string) (string, error) {
	// An input mode can report its default enabled flag while its parent
	// is disabled. Use the effective list before checking registration.
	for _, includeDisabled := range []bool{false, true} {
		sources := t.createList(0, includeDisabled)
		if sources == 0 {
			return "", errors.New("TISCreateInputSourceList failed")
		}
		count := t.count(sources)
		found := false
		for i := 0; i < count; i++ {
			if t.sourceID(t.at(sources, i)) == id {
				found = true
				break
			}
		}
		t.release(sources)
		if found {
			if includeDisabled {
				return "disabled", nil
			}
			return "enabled", nil
		}
	}
	return "missing", nil
}

func (t tis) run(action, argument string) error {
	switch action {
	case "status":
		state, err := t.status(argument)
		if err == nil {
			fmt.Println(state)
		}
		return err
	case "current":
		source := t.current()
		if source == 0 {
			return errors.New("no current keyboard input source")
		}
		defer t.release(source)
		sourceID := t.sourceID(source)
		if sourceID == "" {
			return errors.New("current input source has no ID")
		}
		fmt.Println(sourceID)
		return nil
	case "register":
		path, err := filepath.Abs(argument)
		if err != nil {
			return err
		}
		url := t.fileURL(0, []byte(path), len(path), true)
		if url == 0 {
			return fmt.Errorf("invalid app path %q", path)
		}
		defer t.release(url)
		return checkStatus(action, argument, t.register(url))
	}
	sources := t.createList(0, true)
	if sources == 0 {
		return errors.New("TISCreateInputSourceList failed")
	}
	defer t.release(sources)
	for i, n := 0, t.count(sources); i < n; i++ {
		source := t.at(sources, i)
		if t.sourceID(source) != argument {
			continue
		}
		switch action {
		case "enable":
			return checkStatus(action, argument, t.enable(source))
		case "select":
			return checkStatus(action, argument, t.selectInput(source))
		case "disable":
			return checkStatus(action, argument, t.disable(source))
		}
	}
	return fmt.Errorf("input source %q does not appear in the TIS list", argument)
}

func checkStatus(action, argument string, status int32) error {
	if status != 0 {
		return fmt.Errorf("%s %q failed with OSStatus %d", action, argument, status)
	}
	return nil
}

func main() {
	runtime.LockOSThread()
	log.SetFlags(0)
	action, argument := "", ""
	if len(os.Args) == 2 && os.Args[1] == "current" {
		action = os.Args[1]
	} else if len(os.Args) == 3 {
		switch os.Args[1] {
		case "register", "enable", "select", "disable", "status":
			action, argument = os.Args[1], os.Args[2]
		}
	}
	if action == "" {
		log.Fatal("usage: tis current | tis register APP | tis {enable|select|disable|status} SOURCE_ID")
	}
	t, err := loadTIS()
	if err != nil {
		log.Fatal(err)
	}
	if err := t.run(action, argument); err != nil {
		log.Fatal(err)
	}
}
