//go:build darwin && !ios

package darwin

// Mobile-contract capabilities that macOS also has
// (internal/platform/mobile_ext.go): the share sheet
// (NSSharingServicePicker), Touch ID (LocalAuthentication) and the
// Keychain (Security.framework generic passwords).

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.Sharer      = (*backend)(nil)
	_ platform.Biometrics  = (*backend)(nil)
	_ platform.SecureStore = (*backend)(nil)
)

// ---------------------------------------------------------------------------
// Share sheet

var shareState struct {
	once     sync.Once
	msgShow  func(obj id, s objc.SEL, r nsRect, view id, edge uint)
	pending  map[id]func(error) // delegate -> done
	delegate id
}

func initShare() {
	shareState.once.Do(func() {
		purego.RegisterFunc(&shareState.msgShow, msgSendAddr)
		shareState.pending = map[id]func(error){}
		classDef("TangoSharePickerDelegate", "NSObject", []string{"NSSharingServicePickerDelegate"}, []objc.MethodDef{
			method("sharingServicePicker:didChooseSharingService:", func(self id, _ objc.SEL, picker, service id) {
				done := shareState.pending[self]
				delete(shareState.pending, self)
				release(self)
				if done == nil {
					return
				}
				if service == 0 {
					done(platform.ErrCanceled)
				} else {
					done(nil)
				}
			}),
		})
	})
}

// shareWindow is the key window, else the main window, else any visible one.
func (b *backend) shareWindow() id {
	for _, s := range []string{"keyWindow", "mainWindow"} {
		if w := send(b.app, s); w != 0 && sendBool(w, "isVisible") {
			return w
		}
	}
	for _, w := range arrayItems(send(b.app, "windows")) {
		if sendBool(w, "isVisible") && send(w, "contentView") != 0 {
			return w
		}
	}
	return 0
}

// Share shows NSSharingServicePicker over the key window's content. done(nil)
// once the user picked a service, ErrCanceled when the picker was closed.
func (b *backend) Share(opts platform.ShareOptions, done func(error)) {
	initShare()
	var err error
	withPool(func() {
		w := b.shareWindow()
		if w == 0 {
			err = errors.New("tango/darwin: no visible window to show the share picker from")
			return
		}
		var items []id
		if opts.Text != "" {
			items = append(items, nsString(opts.Text))
		}
		if opts.URL != "" {
			if u := nsURL(opts.URL); u != 0 {
				items = append(items, u)
			}
		}
		for _, p := range opts.Paths {
			items = append(items, fileURL(p))
		}
		if len(items) == 0 {
			err = errors.New("tango/darwin: nothing to share")
			return
		}
		picker := autorelease(send(send(class("NSSharingServicePicker"), "alloc"), "initWithItems:", uintptr(nsArray(items...))))
		del := alloc("TangoSharePickerDelegate") // released when it answers
		shareState.pending[del] = done
		send(picker, "setDelegate:", uintptr(del))
		// The picker closes at once over an inactive app or a non-key window.
		if !sendBool(b.app, "isActive") {
			send(b.app, "activateIgnoringOtherApps:", 1)
		}
		send(w, "makeKeyAndOrderFront:", 0)
		view := send(w, "contentView")
		bounds := msgRect(view, sel("bounds"))
		// The centre of the content; the picker points at it.
		r := nsRect{Origin: nsPoint{X: bounds.Size.Width / 2, Y: bounds.Size.Height / 2}, Size: nsSize{Width: 1, Height: 1}}
		const nsMinYEdge = 1
		shareState.msgShow(picker, sel("showRelativeToRect:ofView:preferredEdge:"), r, view, nsMinYEdge)
	})
	if err != nil {
		done(err)
	}
}

// ---------------------------------------------------------------------------
// Biometrics (LocalAuthentication)

const (
	laPolicyBiometrics  = 1 // LAPolicyDeviceOwnerAuthenticationWithBiometrics
	laPolicyDeviceOwner = 2 // LAPolicyDeviceOwnerAuthentication

	laAuthenticationFailed = -1
	laUserCancel           = -2
	laUserFallback         = -3
	laSystemCancel         = -4
	laPasscodeNotSet       = -5
	laBiometryNotAvailable = -6
	laBiometryNotEnrolled  = -7
	laBiometryLockout      = -8
	laAppCancel            = -9
)

var laState struct {
	once sync.Once
	ok   bool
}

func loadLA() bool {
	laState.once.Do(func() {
		_, err := purego.Dlopen("/System/Library/Frameworks/LocalAuthentication.framework/LocalAuthentication", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		laState.ok = err == nil && hasClass("LAContext")
	})
	return laState.ok
}

// laError maps an LAError code (NSError domain com.apple.LocalAuthentication).
func laError(nsErr id) error {
	if nsErr == 0 {
		return platform.ErrAuthFailed
	}
	switch code := sendInt(nsErr, "code"); code {
	case laUserCancel, laAppCancel, laSystemCancel, laUserFallback:
		return platform.ErrCanceled
	case laAuthenticationFailed:
		return platform.ErrAuthFailed
	case laBiometryNotEnrolled:
		return platform.ErrNotEnrolled
	case laBiometryLockout:
		return platform.ErrLockedOut
	case laBiometryNotAvailable, laPasscodeNotSet:
		return platform.ErrNotAvailable
	case -11, -12, -13: // biometry not paired / disconnected / watch not available (Mac accessories)
		return fmt.Errorf("%w: %s", platform.ErrNotAvailable, goString(send(nsErr, "localizedDescription")))
	default:
		return fmt.Errorf("tango/darwin: %s (LAError %d)", goString(send(nsErr, "localizedDescription")), code)
	}
}

func (b *backend) BiometryKind() (k platform.BiometryKind, err error) {
	if !loadLA() {
		return platform.BiometryNone, platform.ErrNotAvailable
	}
	withPool(func() {
		ctx := autorelease(alloc("LAContext"))
		var nsErr id
		if !sendBool(ctx, "canEvaluatePolicy:error:", laPolicyBiometrics, uintptr(unsafe.Pointer(&nsErr))) {
			k, err = platform.BiometryNone, laError(nsErr)
			if !errors.Is(err, platform.ErrNotEnrolled) && !errors.Is(err, platform.ErrLockedOut) && !errors.Is(err, platform.ErrNotAvailable) {
				err = fmt.Errorf("%w: %v", platform.ErrNotAvailable, err)
			}
			return
		}
		switch sendInt(ctx, "biometryType") {
		case 1:
			k = platform.BiometryTouchID
		case 2:
			k = platform.BiometryFaceID
		case 4:
			k = platform.BiometryOpticID
		default:
			k = platform.BiometryOther
		}
	})
	return
}

func (b *backend) Authenticate(opts platform.BiometricOptions, done func(error)) {
	if !loadLA() {
		done(platform.ErrNotAvailable)
		return
	}
	policy := uintptr(laPolicyBiometrics)
	if opts.AllowPasscode {
		policy = laPolicyDeviceOwner
	}
	var err error
	withPool(func() {
		ctx := alloc("LAContext") // released after the reply
		if opts.CancelTitle != "" {
			send(ctx, "setLocalizedCancelTitle:", uintptr(nsString(opts.CancelTitle)))
		}
		if !opts.AllowPasscode {
			// Hide the "Enter Password" fallback button.
			send(ctx, "setLocalizedFallbackTitle:", uintptr(nsString("")))
		}
		var nsErr id
		if !sendBool(ctx, "canEvaluatePolicy:error:", policy, uintptr(unsafe.Pointer(&nsErr))) {
			err = laError(nsErr)
			if !errors.Is(err, platform.ErrNotEnrolled) && !errors.Is(err, platform.ErrLockedOut) && !errors.Is(err, platform.ErrNotAvailable) {
				err = fmt.Errorf("%w: %v", platform.ErrNotAvailable, err)
			}
			release(ctx)
			return
		}
		reason := opts.Reason
		if reason == "" {
			reason = "Confirm it's you"
		}
		// The reply runs on a private queue.
		reply := objc.NewBlock(func(_ objc.Block, ok bool, nsErr id) {
			var e error
			if !ok {
				e = laError(nsErr)
			}
			mainHop.post(func() {
				release(ctx)
				done(e)
			})
		})
		send(ctx, "evaluatePolicy:localizedReason:reply:", policy, uintptr(nsString(reason)), uintptr(reply))
		reply.Release()
	})
	if err != nil {
		done(err)
	}
}

// ---------------------------------------------------------------------------
// Keychain (any goroutine)

const (
	errSecSuccess      = 0
	errSecItemNotFound = -25300
	errSecDuplicate    = -25299
)

var keychain struct {
	once                                 sync.Once
	err                                  error
	add                                  func(attrs uintptr, result *uintptr) int32
	copyMatching                         func(query uintptr, result *uintptr) int32
	update                               func(query, attrs uintptr) int32
	del                                  func(query uintptr) int32
	errMsg                               func(status int32, reserved uintptr) uintptr
	kClass, kGenericPassword, kService   uintptr
	kAccount, kValueData, kReturnData    uintptr
	kMatchLimit, kMatchLimitOne          uintptr
	kAccessible, kAfterFirstUnlockDevice uintptr
	kTrue                                uintptr
}

func loadKeychain() error {
	k := &keychain
	k.once.Do(func() {
		load()
		lib, err := purego.Dlopen("/System/Library/Frameworks/Security.framework/Security", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			k.err = fmt.Errorf("tango/darwin: Security.framework: %w", err)
			return
		}
		purego.RegisterLibFunc(&k.add, lib, "SecItemAdd")
		purego.RegisterLibFunc(&k.copyMatching, lib, "SecItemCopyMatching")
		purego.RegisterLibFunc(&k.update, lib, "SecItemUpdate")
		purego.RegisterLibFunc(&k.del, lib, "SecItemDelete")
		purego.RegisterLibFunc(&k.errMsg, lib, "SecCopyErrorMessageString")
		for _, c := range []struct {
			dst  *uintptr
			name string
		}{
			{&k.kClass, "kSecClass"}, {&k.kGenericPassword, "kSecClassGenericPassword"},
			{&k.kService, "kSecAttrService"}, {&k.kAccount, "kSecAttrAccount"},
			{&k.kValueData, "kSecValueData"}, {&k.kReturnData, "kSecReturnData"},
			{&k.kMatchLimit, "kSecMatchLimit"}, {&k.kMatchLimitOne, "kSecMatchLimitOne"},
			{&k.kAccessible, "kSecAttrAccessible"}, {&k.kAfterFirstUnlockDevice, "kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly"},
		} {
			if *c.dst = constPtr(lib, c.name); *c.dst == 0 {
				k.err = fmt.Errorf("tango/darwin: Security.framework lacks %s", c.name)
				return
			}
		}
		k.kTrue = constPtr(libCF, "kCFBooleanTrue")
	})
	return k.err
}

func (b *backend) keychainService() string {
	name := b.opts.ID
	if name == "" {
		name = b.opts.Name
	}
	if name == "" {
		name = "tango"
	}
	return name + ".tango.securestore"
}

// keychainQuery is an autoreleased mutable dictionary selecting key's item.
func (b *backend) keychainQuery(key string) id {
	k := &keychain
	q := send(class("NSMutableDictionary"), "dictionary")
	set := func(key uintptr, v id) { send(q, "setObject:forKey:", uintptr(v), key) }
	set(k.kClass, id(k.kGenericPassword))
	set(k.kService, nsString(b.keychainService()))
	set(k.kAccount, nsString(key))
	return q
}

func keychainErr(op string, status int32) error {
	msg := ""
	if keychain.errMsg != nil {
		if s := keychain.errMsg(status, 0); s != 0 {
			msg = goString(id(s))
			release(id(s))
		}
	}
	return fmt.Errorf("tango/darwin: keychain %s: %s (OSStatus %d)", op, msg, status)
}

func (b *backend) SecureSet(key string, value []byte) error {
	if err := loadKeychain(); err != nil {
		return err
	}
	k := &keychain
	var err error
	withPool(func() {
		q := b.keychainQuery(key)
		attrs := send(class("NSMutableDictionary"), "dictionary")
		send(attrs, "setObject:forKey:", uintptr(nsData(value)), k.kValueData)
		st := k.update(uintptr(q), uintptr(attrs))
		if st == errSecItemNotFound {
			send(q, "setObject:forKey:", uintptr(nsData(value)), k.kValueData)
			send(q, "setObject:forKey:", k.kAfterFirstUnlockDevice, k.kAccessible)
			st = k.add(uintptr(q), nil)
		}
		if st != errSecSuccess {
			err = keychainErr("write", st)
		}
	})
	return err
}

func (b *backend) SecureGet(key string) ([]byte, error) {
	if err := loadKeychain(); err != nil {
		return nil, err
	}
	k := &keychain
	var out []byte
	var err error
	withPool(func() {
		q := b.keychainQuery(key)
		send(q, "setObject:forKey:", k.kTrue, k.kReturnData)
		send(q, "setObject:forKey:", k.kMatchLimitOne, k.kMatchLimit)
		var res uintptr
		switch st := k.copyMatching(uintptr(q), &res); st {
		case errSecSuccess:
			out = goBytes(id(res))
			if out == nil {
				out = []byte{}
			}
			release(id(res)) // Copy rule: we own the result
		case errSecItemNotFound:
			err = platform.ErrNotFound
		default:
			err = keychainErr("read", st)
		}
	})
	return out, err
}

func (b *backend) SecureDelete(key string) error {
	if err := loadKeychain(); err != nil {
		return err
	}
	var err error
	withPool(func() {
		if st := keychain.del(uintptr(b.keychainQuery(key))); st != errSecSuccess && st != errSecItemNotFound {
			err = keychainErr("delete", st)
		}
	})
	return err
}
