//go:build windows && (amd64 || arm64)

package windows

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Every Windows dialog is modal: showing it runs a message loop of its own
// until it closes, and that loop keeps dispatching the app's messages
// (Wake / Drain included). The platform contract is asynchronous, so each
// dialog is opened from runDeferred, after the API call has returned, and
// reports through done when its loop ends.

// ownerOf picks the window a dialog belongs to: the requested surface if
// it is open, else the app's active surface, else none.
func (b *Backend) ownerOf(parent platform.SurfaceID) uintptr {
	if s := b.byID[parent]; s != nil && !s.closed {
		return s.hwnd
	}
	active, _, _ := procGetActiveWindow.Call()
	if active == 0 || b.surfaces[active] == nil {
		return 0
	}
	return active
}

func (b *Backend) MessageDialog(parent platform.SurfaceID, opts platform.MessageOptions, done func(button int, err error)) {
	b.runDeferred(func() {
		owner := b.ownerOf(parent)
		labels := messageButtons(opts.Buttons)
		if index, shown, err := b.taskDialog(owner, opts, labels); shown {
			done(index, err)
			return
		}
		// No task dialogs (comctl32 v6 missing): fall back to MessageBoxW
		// and its standard buttons.
		caption := opts.Title
		if caption == "" {
			caption = b.opts.Name
		}
		style := uintptr(messageBoxStyle(opts.Kind, len(labels)))
		if owner == 0 {
			style |= mbTaskModal // still block the app's other windows
		}
		pressed := messageBox(owner, u16(opts.Message), u16(caption), style)
		done(messageBoxIndex(pressed, len(labels)))
	})
}

// messageBox wraps MessageBoxW. Go callbacks run inside its modal loop and
// may move this goroutine's stack; go:uintptrescapes moves the texts to
// the heap so their addresses stay valid throughout.
//
//go:uintptrescapes
func messageBox(owner uintptr, text, title *uint16, style uintptr) uintptr {
	pressed, _, _ := procMessageBoxW.Call(owner, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), style)
	return pressed
}

// comctl32Export resolves a function of the system comctl32.dll through
// LoadLibraryExW + GetProcAddress. Loading it this way, after
// activateCommonControls, binds version 6 (the one with task dialogs);
// LOAD_LIBRARY_SEARCH_SYSTEM32 keeps the search inside System32. It
// returns 0 when either step fails.
func comctl32Export(name string) uintptr {
	const loadLibrarySearchSystem32 = 0x00000800
	module, _, _ := procLoadLibraryExW.Call(uintptr(unsafe.Pointer(u16("comctl32.dll"))), 0, loadLibrarySearchSystem32)
	if module == 0 {
		return 0
	}
	ansiName, err := syscall.BytePtrFromString(name)
	if err != nil {
		return 0
	}
	addr, _, _ := procGetProcAddress.Call(module, uintptr(unsafe.Pointer(ansiName)))
	return addr
}

// taskDialogAddr is TaskDialogIndirect, looked up once; 0 means comctl32
// v6 is not available and MessageBoxW has to do.
var taskDialogAddr = sync.OnceValue(func() uintptr { return comctl32Export("TaskDialogIndirect") })

// TASKDIALOG_FLAGS used (commctrl.h).
const (
	tdfAllowDialogCancellation  = 0x00000008
	tdfPositionRelativeToWindow = 0x00001000
	tdfSizeToContent            = 0x01000000
)

// taskDialog shows a task dialog with one custom button per label and
// returns the chosen index. shown is false if no task dialog appeared
// (comctl32 v6 missing or the call failed); the caller then falls back.
func (b *Backend) taskDialog(owner uintptr, o platform.MessageOptions, labels []string) (index int, shown bool, err error) {
	fn := taskDialogAddr()
	if fn == 0 {
		return 0, false, nil
	}

	// A title becomes the bold main instruction with the message below
	// it; a message alone is the main instruction. The window caption is
	// the app name, else the main instruction.
	instruction, content := o.Title, o.Message
	if instruction == "" {
		instruction, content = o.Message, ""
	}
	caption := b.opts.Name
	if caption == "" {
		caption = instruction
	}

	// texts holds every UTF-16 buffer referenced from the packed
	// structures, which the collector cannot see into.
	texts := make([]*uint16, 0, 3+len(labels))
	text := func(s string) uintptr {
		p := u16(s)
		texts = append(texts, p)
		return uintptr(unsafe.Pointer(p))
	}
	cfg := taskDialogConfig{
		Instance:        instance(),
		Flags:           tdfAllowDialogCancellation | tdfSizeToContent,
		WindowTitle:     text(caption),
		MainIcon:        taskDialogIcon(o.Kind),
		MainInstruction: text(instruction),
		ButtonCount:     uint32(len(labels)),
		DefaultButton:   firstButtonID,
	}
	if contentText := text(content); content != "" {
		cfg.Content = contentText
	}
	if owner != 0 {
		cfg.Parent = owner
		cfg.Flags |= tdfPositionRelativeToWindow
	}
	labelPtrs := make([]uintptr, len(labels))
	for i, l := range labels {
		labelPtrs[i] = text(l)
	}
	buttons := taskDialogButtons(firstButtonID, labelPtrs)
	cfg.Buttons = uintptr(unsafe.Pointer(&buttons[0]))
	packed := cfg.encode()

	var pressed, radio, verified int32
	hr := taskDialogIndirect(fn, uintptr(unsafe.Pointer(&packed[0])), uintptr(unsafe.Pointer(&pressed)),
		uintptr(unsafe.Pointer(&radio)), uintptr(unsafe.Pointer(&verified)))
	runtime.KeepAlive(texts)
	runtime.KeepAlive(buttons)
	if failed(hr) {
		return 0, false, nil
	}
	index, err = taskDialogIndex(pressed, len(labels))
	return index, true, err
}

// taskDialogIndirect calls TaskDialogIndirect at fn. Its modal loop runs
// Go callbacks, so everything it writes to must live on the heap, which
// go:uintptrescapes arranges for the out parameters.
//
//go:uintptrescapes
func taskDialogIndirect(fn, cfg, pressed, radio, verified uintptr) uintptr {
	hr, _, _ := syscall.SyscallN(fn, cfg, pressed, radio, verified)
	return hr
}

// Shell file dialogs (shobjidl_core.h): the class and interface IDs, and
// the interfaces in declaration order for their slot numbers.
var (
	clsidFileOpenDialog = guid("DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7")
	clsidFileSaveDialog = guid("C0B4E2F3-BA21-4773-8DBA-335EC946EB8B")
	iidIFileOpenDialog  = guid("D57C7288-D4AD-4768-BE02-9D969532D960")
	iidIFileSaveDialog  = guid("84BCCD23-5FDE-4CDB-AEA4-AF64B83D78AB")
	iidIShellItem       = guid("43826D1E-E718-42EE-BC55-A1E261C37BFE")
)

// vtIFileOpenDialog lays out IFileOpenDialog, which extends IFileDialog,
// which extends IModalWindow. IFileSaveDialog shares the IFileDialog part.
type vtIFileOpenDialog struct {
	vtIUnknown
	Show                uintptr // IModalWindow
	SetFileTypes        uintptr // IFileDialog from here
	SetFileTypeIndex    uintptr
	GetFileTypeIndex    uintptr
	Advise              uintptr
	Unadvise            uintptr
	SetOptions          uintptr
	GetOptions          uintptr
	SetDefaultFolder    uintptr
	SetFolder           uintptr
	GetFolder           uintptr
	GetCurrentSelection uintptr
	SetFileName         uintptr
	GetFileName         uintptr
	SetTitle            uintptr
	SetOkButtonLabel    uintptr
	SetFileNameLabel    uintptr
	GetResult           uintptr
	AddPlace            uintptr
	SetDefaultExtension uintptr
	Close               uintptr
	SetClientGuid       uintptr
	ClearClientData     uintptr
	SetFilter           uintptr
	GetResults          uintptr // IFileOpenDialog from here
	GetSelectedItems    uintptr
}

type vtIShellItem struct {
	vtIUnknown
	BindToHandler  uintptr
	GetParent      uintptr
	GetDisplayName uintptr
	GetAttributes  uintptr
	Compare        uintptr
}

type vtIShellItemArray struct {
	vtIUnknown
	BindToHandler              uintptr
	GetPropertyStore           uintptr
	GetPropertyDescriptionList uintptr
	GetAttributes              uintptr
	GetCount                   uintptr
	GetItemAt                  uintptr
	EnumItems                  uintptr
}

const (
	fdShow                = int(unsafe.Offsetof(vtIFileOpenDialog{}.Show) / slotSize)
	fdSetFileTypes        = int(unsafe.Offsetof(vtIFileOpenDialog{}.SetFileTypes) / slotSize)
	fdSetOptions          = int(unsafe.Offsetof(vtIFileOpenDialog{}.SetOptions) / slotSize)
	fdGetOptions          = int(unsafe.Offsetof(vtIFileOpenDialog{}.GetOptions) / slotSize)
	fdSetFolder           = int(unsafe.Offsetof(vtIFileOpenDialog{}.SetFolder) / slotSize)
	fdSetFileName         = int(unsafe.Offsetof(vtIFileOpenDialog{}.SetFileName) / slotSize)
	fdSetTitle            = int(unsafe.Offsetof(vtIFileOpenDialog{}.SetTitle) / slotSize)
	fdGetResult           = int(unsafe.Offsetof(vtIFileOpenDialog{}.GetResult) / slotSize)
	fdSetDefaultExtension = int(unsafe.Offsetof(vtIFileOpenDialog{}.SetDefaultExtension) / slotSize)
	fodGetResults         = int(unsafe.Offsetof(vtIFileOpenDialog{}.GetResults) / slotSize)

	siGetDisplayName = int(unsafe.Offsetof(vtIShellItem{}.GetDisplayName) / slotSize)
	siaGetCount      = int(unsafe.Offsetof(vtIShellItemArray{}.GetCount) / slotSize)
	siaGetItemAt     = int(unsafe.Offsetof(vtIShellItemArray{}.GetItemAt) / slotSize)

	sigdnFileSysPath = 0x80058000 // SIGDN_FILESYSPATH
)

// fileDialog is an IFileOpenDialog or IFileSaveDialog pointer, with the
// IFileDialog methods the backend uses.
type fileDialog uintptr

// newFileDialog creates the dialog class clsid through interface iid
// (0 if COM refuses).
func newFileDialog(clsid, iid *GUID) fileDialog {
	var obj uintptr
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&obj)))
	if failed(hr) {
		return 0
	}
	return fileDialog(obj)
}

func (d fileDialog) release() { release(uintptr(d)) }

// addOptions sets FOS_* flags on top of the dialog's defaults.
func (d fileDialog) addOptions(flags uint32) {
	var current uint32
	comCall(uintptr(d), fdGetOptions, uintptr(unsafe.Pointer(&current)))
	comCall(uintptr(d), fdSetOptions, uintptr(current|flags))
}

// setText calls one of the string setters (SetTitle, SetFileName,
// SetDefaultExtension).
func (d fileDialog) setText(slot int, s string) {
	comCall(uintptr(d), slot, uintptr(unsafe.Pointer(u16(s))))
}

// startIn opens the dialog in the folder at path, if the shell can parse
// it into an item.
func (d fileDialog) startIn(path string) {
	var folder uintptr
	hr, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(u16(path))), 0,
		uintptr(unsafe.Pointer(&iidIShellItem)), uintptr(unsafe.Pointer(&folder)))
	if failed(hr) || folder == 0 {
		return
	}
	comCall(uintptr(d), fdSetFolder, folder)
	release(folder)
}

// setFileTypes installs the filter list (COMDLG_FILTERSPEC is a pair of
// string pointers).
func (d fileDialog) setFileTypes(specs []filterSpec) {
	type comdlgFilterSpec struct{ Name, Spec *uint16 }
	native := make([]comdlgFilterSpec, len(specs))
	for i, s := range specs {
		native[i] = comdlgFilterSpec{u16(s.Name), u16(s.Spec)}
	}
	if len(native) != 0 {
		comCall(uintptr(d), fdSetFileTypes, uintptr(len(native)), uintptr(unsafe.Pointer(&native[0])))
	}
}

// configure applies the options of a request before the dialog is shown.
func (d fileDialog) configure(flags uint32, o platform.FileOptions, save bool) {
	d.addOptions(flags)
	if o.Title != "" {
		d.setText(fdSetTitle, o.Title)
	}
	if o.Directory != "" {
		d.startIn(o.Directory)
	}
	if save && o.Filename != "" {
		d.setText(fdSetFileName, o.Filename)
	}
	if o.PickFolders {
		return // folder pickers take no file types
	}
	d.setFileTypes(filterSpecs(o.Filters))
	if ext := defaultExtension(o.Filters); save && ext != "" {
		d.setText(fdSetDefaultExtension, ext)
	}
}

// show runs the dialog's modal loop. Dismissing it is ErrCanceled.
func (d fileDialog) show(owner uintptr) error {
	hr := comCall(uintptr(d), fdShow, owner)
	if uint32(hr) == errorCancelledHRes {
		return platform.ErrCanceled
	}
	if failed(hr) {
		return hresultError("IFileDialog::Show", hr)
	}
	return nil
}

// shellItemPath returns an IShellItem's file system path ("" if it has
// none).
func shellItemPath(item uintptr) string {
	var p uintptr
	if hr := comCall(item, siGetDisplayName, sigdnFileSysPath, uintptr(unsafe.Pointer(&p))); failed(hr) {
		return ""
	}
	return takeWstr(p)
}

// shellItemArrayPaths returns the paths of the items in an
// IShellItemArray, skipping items that cannot be read or have no path.
func shellItemArrayPaths(items uintptr) []string {
	var count uint32
	comCall(items, siaGetCount, uintptr(unsafe.Pointer(&count)))
	var paths []string
	for i := range count {
		var item uintptr
		if comCall(items, siaGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&item))) != sOK || item == 0 {
			continue
		}
		if p := shellItemPath(item); p != "" {
			paths = append(paths, p)
		}
		release(item)
	}
	return paths
}

func (b *Backend) OpenFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(paths []string, err error)) {
	b.runDeferred(func() {
		d := newFileDialog(&clsidFileOpenDialog, &iidIFileOpenDialog)
		if d == 0 {
			done(nil, platform.ErrUnsupported)
			return
		}
		defer d.release()
		d.configure(openDialogOptions(opts), opts, false)
		if err := d.show(b.ownerOf(parent)); err != nil {
			done(nil, err)
			return
		}
		var items uintptr
		hr := comCall(uintptr(d), fodGetResults, uintptr(unsafe.Pointer(&items)))
		if failed(hr) || items == 0 {
			done(nil, hresultError("IFileOpenDialog::GetResults", hr))
			return
		}
		defer release(items)
		done(shellItemArrayPaths(items), nil)
	})
}

func (b *Backend) SaveFileDialog(parent platform.SurfaceID, opts platform.FileOptions, done func(path string, err error)) {
	b.runDeferred(func() {
		d := newFileDialog(&clsidFileSaveDialog, &iidIFileSaveDialog)
		if d == 0 {
			done("", platform.ErrUnsupported)
			return
		}
		defer d.release()
		d.configure(saveDialogOptions(opts), opts, true)
		if err := d.show(b.ownerOf(parent)); err != nil {
			done("", err)
			return
		}
		var item uintptr
		hr := comCall(uintptr(d), fdGetResult, uintptr(unsafe.Pointer(&item)))
		if failed(hr) || item == 0 {
			done("", hresultError("IFileDialog::GetResult", hr))
			return
		}
		defer release(item)
		done(shellItemPath(item), nil)
	})
}
