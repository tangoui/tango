//go:build windows && (amd64 || arm64)

package windows

// platform.AppShortcuts on Windows: the "Tasks" category of the taskbar
// Jump List, through ICustomDestinationList. Each task is an IShellLinkW
// that starts this executable with platform.ShortcutArgPrefix + ID; its
// title is set through IPropertyStore (PKEY_Title), separators are links
// with PKEY_AppUserModel_IsDestListSeparator. The list belongs to the
// process AppUserModelID (initSysx) and is kept by the shell across runs.
// A click starts a new process; the backend never calls onClick.

import (
	"errors"
	"os"
	"runtime"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.AppShortcuts = (*Backend)(nil)

var (
	sxCLSIDDestinationList    = guid("77f10cf0-3db5-4966-b520-b7c54fd35ed6")
	sxIIDICustomDestList      = guid("6332debf-87b5-4670-90c0-5e57b408a49e")
	sxCLSIDEnumObjCollection  = guid("2d3468c1-36a7-43b6-ac24-d3f02fd9607a")
	sxIIDIObjectCollection    = guid("5632b1a4-e38a-400a-928a-d4cd63230295")
	sxIIDIObjectArray         = guid("92ca9dcd-5622-4bba-a805-5e9f541bd8c9")
	sxCLSIDShellLink          = guid("00021401-0000-0000-c000-000000000046")
	sxIIDIShellLinkW          = guid("000214f9-0000-0000-c000-000000000046")
	sxIIDIPropertyStore       = guid("886d8eeb-8cf2-4446-8d02-cdba1dbdcf99")
	sxPKeyTitle               = sxPropertyKey{guid("f29f85e0-4ff9-1068-ab91-08002b27b3d9"), 2}
	sxPKeyIsDestListSeparator = sxPropertyKey{guid("9f4c2855-9f79-4b39-a8d0-e1d42de1d5f3"), 6}
)

const (
	// ICustomDestinationList
	sxCDLSetAppID     = 3
	sxCDLBeginList    = 4
	sxCDLAddUserTasks = 7
	sxCDLCommitList   = 8
	sxCDLDeleteList   = 10
	sxCDLAbortList    = 11
	// IObjectCollection (IObjectArray: GetCount 3, GetAt 4)
	sxOCAddObject = 5
	// IShellLinkW
	sxSLSetDescription  = 7
	sxSLSetArguments    = 11
	sxSLSetIconLocation = 17
	sxSLSetPath         = 20
	// IPropertyStore
	sxPSSetValue = 6
	sxPSCommit   = 7

	sxVTBool   = 11
	sxVTLPWStr = 31
)

type sxPropertyKey struct {
	Fmtid GUID
	Pid   uint32
}

// sxPropVariant is PROPVARIANT (24 bytes on 64-bit).
type sxPropVariant struct {
	VT  uint16
	_   [3]uint16
	Val uintptr
	_   uintptr
}

var _ = [1]struct{}{}[unsafe.Sizeof(sxPropVariant{})-24]

func (b *Backend) SetAppShortcuts(items []platform.ShortcutItem, _ func(string)) error {
	if sxAppID == "" {
		return errors.New("tango: the Jump List needs an app ID or name (AppUserModelID)")
	}
	cdl, hr := sxCoCreate(&sxCLSIDDestinationList, &sxIIDICustomDestList)
	if cdl == 0 {
		return hresultError("CoCreateInstance(DestinationList)", hr)
	}
	defer release(cdl)
	appID := utf16z(sxAppID)
	if hr := comCall(cdl, sxCDLSetAppID, uintptr(unsafe.Pointer(&appID[0]))); failed(hr) {
		return hresultError("ICustomDestinationList::SetAppID", hr)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	tasks, err := sxTasks(items, exe)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		if hr := comCall(cdl, sxCDLDeleteList, uintptr(unsafe.Pointer(&appID[0]))); failed(hr) {
			return hresultError("ICustomDestinationList::DeleteList", hr)
		}
		return nil
	}
	var slots uint32
	var removed uintptr
	if hr := comCall(cdl, sxCDLBeginList, uintptr(unsafe.Pointer(&slots)), uintptr(unsafe.Pointer(&sxIIDIObjectArray)), uintptr(unsafe.Pointer(&removed))); failed(hr) {
		return hresultError("ICustomDestinationList::BeginList", hr)
	}
	release(removed)
	committed := false
	defer func() {
		if !committed {
			comCall(cdl, sxCDLAbortList)
		}
	}()
	coll, hr := sxCoCreate(&sxCLSIDEnumObjCollection, &sxIIDIObjectCollection)
	if coll == 0 {
		return hresultError("CoCreateInstance(EnumerableObjectCollection)", hr)
	}
	defer release(coll)
	for _, t := range tasks {
		link, err := sxTaskLink(t, exe)
		if err != nil {
			return err
		}
		hr := comCall(coll, sxOCAddObject, link)
		release(link)
		if failed(hr) {
			return hresultError("IObjectCollection::AddObject", hr)
		}
	}
	arr := queryInterface(coll, &sxIIDIObjectArray)
	if arr == 0 {
		return errors.New("tango: IObjectCollection lacks IObjectArray")
	}
	defer release(arr)
	if hr := comCall(cdl, sxCDLAddUserTasks, arr); failed(hr) {
		return hresultError("ICustomDestinationList::AddUserTasks", hr)
	}
	if hr := comCall(cdl, sxCDLCommitList); failed(hr) {
		return hresultError("ICustomDestinationList::CommitList", hr)
	}
	committed = true
	return nil
}

// sxTaskLink makes the IShellLinkW of one task (caller releases).
func sxTaskLink(t sxTask, exe string) (uintptr, error) {
	link, hr := sxCoCreate(&sxCLSIDShellLink, &sxIIDIShellLinkW)
	if link == 0 {
		return 0, hresultError("CoCreateInstance(ShellLink)", hr)
	}
	ok := false
	defer func() {
		if !ok {
			release(link)
		}
	}()
	ps := queryInterface(link, &sxIIDIPropertyStore)
	if ps == 0 {
		return 0, errors.New("tango: ShellLink lacks IPropertyStore")
	}
	defer release(ps)
	if t.separator {
		pv := sxPropVariant{VT: sxVTBool, Val: 0xFFFF} // VARIANT_TRUE
		if hr := comCall(ps, sxPSSetValue, uintptr(unsafe.Pointer(&sxPKeyIsDestListSeparator)), uintptr(unsafe.Pointer(&pv))); failed(hr) {
			return 0, hresultError("IPropertyStore::SetValue(IsDestListSeparator)", hr)
		}
	} else {
		comCall(link, sxSLSetPath, uintptr(unsafe.Pointer(u16(exe))))
		comCall(link, sxSLSetArguments, uintptr(unsafe.Pointer(u16(t.args))))
		if t.description != "" {
			comCall(link, sxSLSetDescription, uintptr(unsafe.Pointer(u16(t.description))))
		}
		comCall(link, sxSLSetIconLocation, uintptr(unsafe.Pointer(u16(t.iconPath))), uintptr(int32(t.iconIndex)))
		title := utf16z(t.title)
		pv := sxPropVariant{VT: sxVTLPWStr, Val: uintptr(unsafe.Pointer(&title[0]))}
		hr := comCall(ps, sxPSSetValue, uintptr(unsafe.Pointer(&sxPKeyTitle)), uintptr(unsafe.Pointer(&pv)))
		runtime.KeepAlive(title)
		if failed(hr) {
			return 0, hresultError("IPropertyStore::SetValue(Title)", hr)
		}
	}
	if hr := comCall(ps, sxPSCommit); failed(hr) {
		return 0, hresultError("IPropertyStore::Commit", hr)
	}
	ok = true
	return link, nil
}
