//go:build windows && (amd64 || arm64)

package windows

// Windows toast notifications through WinRT
// (Windows.UI.Notifications.ToastNotificationManager), with the
// notification-area balloon of notifyhotkey_windows.go as the fallback.
//
// A toast is shown for the process AppUserModelID (initSysx:
// SetCurrentProcessExplicitAppUserModelID). Windows only shows toasts of
// an AUMID it knows: a packaged app, a Start-menu shortcut carrying
// System.AppUserModel.ID (what an installer writes), or — for unpackaged
// apps, honoured by current Windows 10 and Windows 11 builds — the
// per-user key HKCU\Software\Classes\AppUserModelId\<AUMID> with a
// DisplayName, which is written here before the first toast. On systems
// that honour none of these (older Windows 10 without a shortcut) Show
// succeeds but nothing appears and toasts never reach the Action Center;
// such systems usually report the notifier's Setting as not Enabled, and
// then, or whenever WinRT is unavailable or any step fails, the balloon is
// used instead. Clicks on a toast reach onClick while this process runs
// (Activated event); a click on a toast of an earlier run cannot launch
// the app (that needs a registered COM activator, i.e. an installer).
//
// TANGO_WINDOWS_TOAST=0 forces balloons (for testing).

import (
	"errors"
	"os"
	"sync"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var (
	sxProcRegCreateKeyExW = advapi32.NewProc("RegCreateKeyExW")
	sxProcRegSetValueExW  = advapi32.NewProc("RegSetValueExW")
)

var (
	sxIIDIToastManagerStatics = guid("50ac103f-d235-4598-bbef-98fe4d1a3ad4")
	sxIIDIToastFactory        = guid("04124b20-82c6-4229-b109-fd9ed4662b53")
	sxIIDIXmlDocumentIO       = guid("6cd0e74e-ee65-4489-9ebf-ca43e87ba637")
	sxIIDIXmlDocument         = guid("f7f3a506-1e87-42d6-bcfb-b8c809fa5494")
	// TypedEventHandler<ToastNotification, IInspectable>
	sxIIDToastActivatedHandler = guid("ab54de2d-97d9-5528-b6ad-105afe156530")
)

const (
	// IToastNotificationManagerStatics (IInspectable: 3-5)
	sxTNMCreateToastNotifierWithID = 7
	// IToastNotifier
	sxTNShow       = 6
	sxTNGetSetting = 8
	// IXmlDocumentIO
	sxXDLoadXML = 6
	// IToastNotificationFactory
	sxTFCreate = 6
	// IToastNotification
	sxTOAddActivated = 11

	sxMaxLiveToasts = 32
)

var errSxNoToast = errors.New("tango: toast notifications unavailable")

var sxToasts struct {
	once     sync.Once
	ok       bool
	notifier uintptr // IToastNotifier
	factory  uintptr // IToastNotificationFactory
	live     []uintptr
}

func (b *Backend) sxToastSetup() bool {
	sxToasts.once.Do(func() {
		if os.Getenv("TANGO_WINDOWS_TOAST") == "0" || sxAppID == "" || !sxWinRT() {
			return
		}
		sxRegisterAUMID(sxAppID, sxAppKey("", b.opts.Name))
		statics, _ := sxActivationFactory("Windows.UI.Notifications.ToastNotificationManager", &sxIIDIToastManagerStatics)
		if statics == 0 {
			return
		}
		defer release(statics)
		h := sxHString(sxAppID)
		defer sxFreeHString(h)
		var notifier uintptr
		if failed(comCall(statics, sxTNMCreateToastNotifierWithID, h, uintptr(unsafe.Pointer(&notifier)))) || notifier == 0 {
			return
		}
		factory, _ := sxActivationFactory("Windows.UI.Notifications.ToastNotification", &sxIIDIToastFactory)
		if factory == 0 {
			release(notifier)
			return
		}
		sxToasts.notifier, sxToasts.factory, sxToasts.ok = notifier, factory, true
	})
	if !sxToasts.ok {
		return false
	}
	// NotificationSetting: 0 Enabled; DisabledForApplication/ForUser/
	// ByGroupPolicy/ByManifest otherwise. Re-read every time: the user
	// may change it. Reading it fails with ERROR_NOT_FOUND (0x80070490) for
	// an unpackaged app without a Start-menu shortcut, whose toasts still
	// show (the AUMID is registered above): only a setting that was read
	// and is not Enabled turns toasts off.
	var setting int32
	if hr := comCall(sxToasts.notifier, sxTNGetSetting, uintptr(unsafe.Pointer(&setting))); !failed(hr) && setting != 0 {
		return false
	}
	return true
}

// sxRegisterAUMID writes HKCU\Software\Classes\AppUserModelId\<aumid>
// DisplayName, which lets current Windows show toasts of an unpackaged
// app without a Start-menu shortcut.
func sxRegisterAUMID(aumid, displayName string) {
	if !has(sxProcRegCreateKeyExW) {
		return
	}
	var key uintptr
	r, _, _ := sxProcRegCreateKeyExW.Call(hkeyCurrentUser, uintptr(unsafe.Pointer(u16(`Software\Classes\AppUserModelId\`+aumid))),
		0, 0, 0, 0x20006 /* KEY_WRITE */, 0, uintptr(unsafe.Pointer(&key)), 0)
	if r != 0 {
		return
	}
	defer procRegCloseKey.Call(key)
	v := utf16z(displayName)
	sxProcRegSetValueExW.Call(key, uintptr(unsafe.Pointer(u16("DisplayName"))), 0, 1, /* REG_SZ */
		uintptr(unsafe.Pointer(&v[0])), uintptr(2*len(v)))
}

// sxToast shows n as a toast; an error means the caller should fall back.
// Main thread.
func (b *Backend) sxToast(n platform.Notification, onClick func()) error {
	if !b.sxToastSetup() {
		return errSxNoToast
	}
	insp, _ := sxActivate("Windows.Data.Xml.Dom.XmlDocument")
	if insp == 0 {
		return errSxNoToast
	}
	defer release(insp)
	io := queryInterface(insp, &sxIIDIXmlDocumentIO)
	if io == 0 {
		return errSxNoToast
	}
	defer release(io)
	x := sxHString(sxToastXML(n, b.opts.Name))
	hr := comCall(io, sxXDLoadXML, x)
	sxFreeHString(x)
	if failed(hr) {
		return hresultError("XmlDocument.LoadXml", hr)
	}
	doc := queryInterface(insp, &sxIIDIXmlDocument)
	if doc == 0 {
		return errSxNoToast
	}
	defer release(doc)
	var toast uintptr
	if hr := comCall(sxToasts.factory, sxTFCreate, doc, uintptr(unsafe.Pointer(&toast))); failed(hr) || toast == 0 {
		return hresultError("ToastNotification constructor", hr)
	}
	if onClick != nil {
		// Activated arrives on a thread-pool thread.
		d := sxNewDelegate(sxIIDToastActivatedHandler, func(_, _ uintptr) { sxPost(onClick) })
		var tok int64
		comCall(toast, sxTOAddActivated, d, uintptr(unsafe.Pointer(&tok)))
		release(d)
	}
	if hr := comCall(sxToasts.notifier, sxTNShow, toast); failed(hr) {
		release(toast)
		return hresultError("ToastNotifier.Show", hr)
	}
	// Keep recent toasts alive so clicks from the Action Center still
	// reach their Activated handler while the app runs.
	sxToasts.live = append(sxToasts.live, toast)
	if len(sxToasts.live) > sxMaxLiveToasts {
		release(sxToasts.live[0])
		sxToasts.live = sxToasts.live[1:]
	}
	return nil
}
