//go:build windows && (amd64 || arm64)

package windows

// platform.FileDragger on Windows: OLE drag and drop with DoDragDrop.
//
// The data object is the shell's own for the files (SHParseDisplayName →
// SHCreateShellItemArrayFromIDLists → BindToHandler(BHID_DataObject)),
// which offers every format Explorer and other apps look for (Shell IDList
// Array, CF_HDROP, file names) and supports the drag image helper. If the
// shell cannot make one (paths it cannot parse), a minimal Go IDataObject
// offering CF_HDROP is used instead. The drop source is ours: Esc or the
// other mouse button cancels, releasing the button that started the drag
// drops. DoDragDrop runs a modal loop on the main thread; StartFileDrag
// returns after done has run. Allowed effects are copy and link (never
// move: the dragged files belong to the app). The icon argument is not
// used; the shell draws its own drag image.

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.FileDragger = (*surface)(nil)

var (
	sxProcDoDragDrop                     = ole32.NewProc("DoDragDrop")
	sxProcSHParseDisplayName             = shell32.NewProc("SHParseDisplayName")
	sxProcSHCreateShellItemArrayFromIDLs = shell32.NewProc("SHCreateShellItemArrayFromIDLists")
	sxProcSHCreateStdEnumFmtEtc          = shell32.NewProc("SHCreateStdEnumFmtEtc")
)

var (
	sxIIDIDataObject = guid("0000010e-0000-0000-c000-000000000046")
	sxIIDIDropSource = guid("00000121-0000-0000-c000-000000000046")
	sxBHIDDataObject = guid("b8c0bd9f-ed24-455c-83e6-d5390c4fe8c4")
)

const (
	sxVKLButton = 0x01
	sxVKRButton = 0x02

	sxDVAspectContent = 1
	sxTymedHGlobal    = 1

	sxDVEFormatEtc           = 0x80040064
	sxOLEEAdviseNotSupported = 0x80040003
	sxDataSSameFormatEtc     = 0x00040130
	sxENotImpl               = 0x80004001
	sxDataDirGet             = 1
)

// sxFormatEtc is FORMATETC; sxStgMedium STGMEDIUM.
type sxFormatEtc struct {
	CfFormat uint16
	_        [6]byte
	Ptd      uintptr
	Aspect   uint32
	Lindex   int32
	Tymed    uint32
	_        uint32
}

type sxStgMedium struct {
	Tymed   uint32
	_       uint32
	Data    uintptr
	UnkRels uintptr
}

var (
	_ = [1]struct{}{}[unsafe.Sizeof(sxFormatEtc{})-32]
	_ = [1]struct{}{}[unsafe.Sizeof(sxStgMedium{})-24]
)

func sxButtonDown(vk uintptr) bool {
	r, _, _ := sxProcGetAsyncKeyState.Call(vk)
	return int16(r) < 0
}

// sxPressedButton is the MK_ button the user holds now (left or right,
// honouring swapped buttons), or 0.
func sxPressedButton() uint32 {
	swapped, _, _ := sxProcGetSystemMetrics.Call(23) // SM_SWAPBUTTON
	l, r := uintptr(sxVKLButton), uintptr(sxVKRButton)
	if swapped != 0 {
		l, r = r, l
	}
	switch {
	case sxButtonDown(l):
		return sxMKLButton
	case sxButtonDown(r):
		return sxMKRButton
	}
	return 0
}

var errSxDragging = errors.New("tango: a drag is already in progress")

var sxDragging bool

func (s *surface) StartFileDrag(paths []string, icon []byte, done func(platform.DragOperation)) error {
	if s.closed || s.hwnd == 0 {
		return errors.New("tango: surface closed")
	}
	if sxDragging {
		return errSxDragging
	}
	button := sxPressedButton()
	if button == 0 {
		// No press to tie the drag to: it would drop at once wherever the
		// cursor is.
		done(platform.DragNone)
		return nil
	}
	sxOle()
	data := sxShellDataObject(paths)
	if data == 0 {
		hdrop, err := dropFiles(paths)
		if err != nil {
			return err
		}
		data = sxNewHDropObject(hdrop)
	}
	src := sxNewDropSource(button)
	sxDragging = true
	var effect uint32
	hr, _, _ := sxProcDoDragDrop.Call(data, src, sxDropEffectCopy|sxDropEffectLink, uintptr(unsafe.Pointer(&effect)))
	sxDragging = false
	release(src)
	release(data)
	if failed(hr) {
		return hresultError("DoDragDrop", hr)
	}
	done(sxDragOperation(hr == sxDragDropSDrop, effect))
	return nil
}

// sxShellDataObject asks the shell for a data object of paths (0 if it
// cannot make one).
func sxShellDataObject(paths []string) uintptr {
	if !has(sxProcSHCreateShellItemArrayFromIDLs) {
		return 0
	}
	pidls := make([]uintptr, 0, len(paths))
	defer func() {
		for _, p := range pidls {
			procCoTaskMemFree.Call(p)
		}
	}()
	for _, p := range paths {
		var pidl uintptr
		hr, _, _ := sxProcSHParseDisplayName.Call(uintptr(unsafe.Pointer(u16(p))), 0, uintptr(unsafe.Pointer(&pidl)), 0, 0)
		if failed(hr) || pidl == 0 {
			return 0
		}
		pidls = append(pidls, pidl)
	}
	var arr uintptr
	hr, _, _ := sxProcSHCreateShellItemArrayFromIDLs.Call(uintptr(len(pidls)), uintptr(unsafe.Pointer(&pidls[0])), uintptr(unsafe.Pointer(&arr)))
	if failed(hr) || arr == 0 {
		return 0
	}
	defer release(arr)
	var data uintptr
	// IShellItemArray::BindToHandler(pbc, bhid, riid, ppv)
	if failed(comCall(arr, 3, 0, uintptr(unsafe.Pointer(&sxBHIDDataObject)), uintptr(unsafe.Pointer(&sxIIDIDataObject)), uintptr(unsafe.Pointer(&data)))) {
		return 0
	}
	return data
}

// ---- IDropSource ----

var (
	sxDropSourceOnce sync.Once
	sxDropSourceVtbl []uintptr
)

type sxDropSourceState struct{ button uint32 }

func sxNewDropSource(button uint32) uintptr {
	sxDropSourceOnce.Do(func() {
		sxDropSourceVtbl = sxVtable(
			// QueryContinueDrag(BOOL fEscapePressed, DWORD grfKeyState)
			syscall.NewCallback(func(this, escape, keys uintptr) uintptr {
				st, _ := sxState[*sxDropSourceState](this)
				button := uint32(sxMKLButton)
				if st != nil {
					button = st.button
				}
				return sxQueryContinue(uint32(escape) != 0, uint32(keys), button)
			}),
			// GiveFeedback(DWORD dwEffect)
			syscall.NewCallback(func(this, effect uintptr) uintptr { return sxDragDropSUseDefault }),
		)
	})
	return sxNewObject(sxDropSourceVtbl, []GUID{sxIIDIDropSource}, false, &sxDropSourceState{button})
}

// ---- Minimal IDataObject with CF_HDROP ----

var (
	sxHDropOnce sync.Once
	sxHDropVtbl []uintptr
)

type sxHDropState struct{ data []byte }

func sxHDropFormat(f *sxFormatEtc) bool {
	return f.CfFormat == cfHDROP && f.Tymed&sxTymedHGlobal != 0 && (f.Aspect == 0 || f.Aspect == sxDVAspectContent)
}

func sxNewHDropObject(data []byte) uintptr {
	sxHDropOnce.Do(func() {
		sxHDropVtbl = sxVtable(
			// GetData(FORMATETC*, STGMEDIUM*)
			syscall.NewCallback(func(this, pf, pm uintptr) uintptr {
				st, ok := sxState[*sxHDropState](this)
				if !ok || pf == 0 || pm == 0 || !sxHDropFormat((*sxFormatEtc)(native(pf))) {
					return sxDVEFormatEtc
				}
				h, err := globalCopy(st.data)
				if err != nil {
					return 0x8007000E // E_OUTOFMEMORY
				}
				m := (*sxStgMedium)(native(pm))
				m.Tymed, m.Data, m.UnkRels = sxTymedHGlobal, h, 0
				return sOK
			}),
			// GetDataHere
			syscall.NewCallback(func(this, pf, pm uintptr) uintptr { return sxENotImpl }),
			// QueryGetData(FORMATETC*)
			syscall.NewCallback(func(this, pf uintptr) uintptr {
				if pf != 0 && sxHDropFormat((*sxFormatEtc)(native(pf))) {
					return sOK
				}
				return sxDVEFormatEtc
			}),
			// GetCanonicalFormatEtc(in, out)
			syscall.NewCallback(func(this, in, out uintptr) uintptr {
				if out != 0 {
					(*sxFormatEtc)(native(out)).Ptd = 0
				}
				return sxDataSSameFormatEtc
			}),
			// SetData(FORMATETC*, STGMEDIUM*, BOOL)
			syscall.NewCallback(func(this, pf, pm, rel uintptr) uintptr { return sxENotImpl }),
			// EnumFormatEtc(DWORD, IEnumFORMATETC**)
			syscall.NewCallback(func(this, dir, out uintptr) uintptr {
				if dir != sxDataDirGet || out == 0 {
					return sxENotImpl
				}
				f := sxFormatEtc{CfFormat: cfHDROP, Aspect: sxDVAspectContent, Lindex: -1, Tymed: sxTymedHGlobal}
				hr, _, _ := sxProcSHCreateStdEnumFmtEtc.Call(1, uintptr(unsafe.Pointer(&f)), out)
				return hr
			}),
			// DAdvise, DUnadvise, EnumDAdvise
			syscall.NewCallback(func(this, a, b, c, d uintptr) uintptr { return sxOLEEAdviseNotSupported }),
			syscall.NewCallback(func(this, a uintptr) uintptr { return sxOLEEAdviseNotSupported }),
			syscall.NewCallback(func(this, a uintptr) uintptr { return sxOLEEAdviseNotSupported }),
		)
	})
	return sxNewObject(sxHDropVtbl, []GUID{sxIIDIDataObject}, false, &sxHDropState{data})
}
