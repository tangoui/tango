//go:build windows && (amd64 || arm64)

package windows

import (
	"slices"
	"testing"
	"unsafe"
)

// These checks need a real Windows (CI): they resolve every declared entry
// point and drive the COM callback objects through their vtable.

// TestDeclaredProcNames compares the functions win32_windows.go declares
// with their names as spelled in the Windows SDK, library by library.
func TestDeclaredProcNames(t *testing.T) {
	want := map[*sysLib][]string{
		kernel32: {
			"GetSystemDirectoryW", "GetModuleHandleW", "LoadLibraryExW", "GetProcAddress",
			"CreateActCtxW", "ActivateActCtx", "GlobalAlloc", "GlobalLock", "GlobalUnlock",
			"GlobalSize", "GlobalFree", "GetUserPreferredUILanguages",
		},
		user32: {
			"SetProcessDpiAwarenessContext", "SetProcessDPIAware", "GetDpiForWindow",
			"GetDpiForSystem", "AdjustWindowRectExForDpi", "AdjustWindowRectEx",
			"RegisterClassExW", "LoadCursorW", "LoadIconW", "CreateWindowExW", "DestroyWindow",
			"DefWindowProcW", "ShowWindow", "UpdateWindow", "IsIconic", "SetWindowPos",
			"GetWindowRect", "GetClientRect", "SetWindowTextW", "SetForegroundWindow",
			"GetForegroundWindow", "GetActiveWindow", "GetAncestor", "MonitorFromWindow",
			"GetMonitorInfoW", "GetMessageW", "PeekMessageW", "TranslateMessage",
			"DispatchMessageW", "PostMessageW", "PostQuitMessage", "GetKeyState", "MessageBoxW",
			"CreateMenu", "CreatePopupMenu", "AppendMenuW", "SetMenu", "GetMenu", "DrawMenuBar",
			"DestroyMenu", "CreateAcceleratorTableW", "TranslateAcceleratorW",
			"DestroyAcceleratorTable", "OpenClipboard", "IsClipboardFormatAvailable",
			"GetClipboardData", "EmptyClipboard", "SetClipboardData", "CloseClipboard",
		},
		ole32:    {"CoInitializeEx", "CoCreateInstance", "CoTaskMemFree"},
		shell32:  {"SHCreateItemFromParsingName"},
		shlwapi:  {"SHCreateMemStream"},
		advapi32: {"RegOpenKeyExW", "RegQueryValueExW", "RegCloseKey"},
	}
	for lib, names := range want {
		var declared []string
		for _, p := range lib.declared() {
			declared = append(declared, p.name)
		}
		for _, name := range names {
			if !slices.Contains(declared, name) {
				t.Errorf("%s: %s is not declared", lib.path, name)
			}
		}
	}
	// The package variables point at the declarations checked above.
	for _, c := range []struct {
		p    *sysProc
		lib  *sysLib
		name string
	}{
		{procLoadIconW, user32, "LoadIconW"},
		{procGetMessageW, user32, "GetMessageW"},
		{procCoTaskMemFree, ole32, "CoTaskMemFree"},
		{procSHCreateMemStream, shlwapi, "SHCreateMemStream"},
		{procRegQueryValueExW, advapi32, "RegQueryValueExW"},
		{procGlobalAlloc, kernel32, "GlobalAlloc"},
	} {
		if c.p.lib != c.lib || c.p.name != c.name {
			t.Errorf("%s declared as %s!%s", c.name, c.p.lib.path, c.p.name)
		}
	}
}

// TestDeclaredProcsResolve looks up every function declared through a
// sysLib, in any file, on the running system. A misspelt name fails here.
func TestDeclaredProcsResolve(t *testing.T) {
	n := 0
	for _, lib := range sysLibs {
		for _, p := range lib.declared() {
			n++
			if err := p.Find(); err != nil {
				t.Errorf("%s!%s: %v", lib.path, p.name, err)
			}
			if !p.Available() || !has(p) {
				t.Errorf("%s!%s: not available", lib.path, p.name)
			}
		}
	}
	if n < 60 {
		t.Errorf("only %d functions declared", n)
	}
	missing := &sysProc{lib: kernel32, name: "TangoNoSuchFunction"} // not in the inventory
	if missing.Available() || has(missing) {
		t.Error("a missing export reads as available")
	}
	defer func() {
		if recover() == nil {
			t.Error("Call of a missing export did not panic")
		}
	}()
	missing.Call()
}

// TestSlotConstants checks the vtable slot constants against the method
// order of WebView2.h and shobjidl_core.h, counted by hand (IUnknown takes
// slots 0-2).
func TestSlotConstants(t *testing.T) {
	for _, c := range []struct {
		name      string
		got, want int
	}{
		{"IUnknown::QueryInterface", unkQueryInterface, 0},
		{"IUnknown::AddRef", unkAddRef, 1},
		{"IUnknown::Release", unkRelease, 2},
		{"ISequentialStream::Read", seqStreamRead, 3},

		{"ICoreWebView2Environment::CreateCoreWebView2Controller", envCreateController, 3},
		{"ICoreWebView2Environment::CreateWebResourceResponse", envCreateWebResourceResponse, 4},

		{"ICoreWebView2Controller::put_IsVisible", ctlPutIsVisible, 4},
		{"ICoreWebView2Controller::put_Bounds", ctlPutBounds, 6},
		{"ICoreWebView2Controller::MoveFocus", ctlMoveFocus, 12},
		{"ICoreWebView2Controller::add_AcceleratorKeyPressed", ctlAddAcceleratorKeyPressed, 19},
		{"ICoreWebView2Controller::NotifyParentWindowPositionChanged", ctlNotifyParentWindowPositionChanged, 23},
		{"ICoreWebView2Controller::Close", ctlClose, 24},
		{"ICoreWebView2Controller::get_CoreWebView2", ctlGetCoreWebView2, 25},

		{"ICoreWebView2::get_Settings", wvGetSettings, 3},
		{"ICoreWebView2::get_Source", wvGetSource, 4},
		{"ICoreWebView2::Navigate", wvNavigate, 5},
		{"ICoreWebView2::add_NavigationCompleted", wvAddNavigationCompleted, 15},
		{"ICoreWebView2::AddScriptToExecuteOnDocumentCreated", wvAddScriptToExecuteOnDocumentCreated, 27},
		{"ICoreWebView2::ExecuteScript", wvExecuteScript, 29},
		{"ICoreWebView2::add_WebMessageReceived", wvAddWebMessageReceived, 34},
		{"ICoreWebView2::CallDevToolsProtocolMethod", wvCallDevToolsProtocolMethod, 36},
		{"ICoreWebView2::OpenDevToolsWindow", wvOpenDevToolsWindow, 51},
		{"ICoreWebView2::add_WebResourceRequested", wvAddWebResourceRequested, 55},
		{"ICoreWebView2::AddWebResourceRequestedFilter", wvAddWebResourceRequestedFilter, 57},
		// Constants declared by hand elsewhere must agree with the layout.
		{"ICoreWebView2::add_NavigationStarting", int(unsafe.Offsetof(vtICoreWebView2{}.AddNavigationStarting) / slotSize), wvAddNavigationStarting},
		{"ICoreWebView2::Reload", int(unsafe.Offsetof(vtICoreWebView2{}.Reload) / slotSize), wvReload},
		{"ICoreWebView2::add_PermissionRequested", int(unsafe.Offsetof(vtICoreWebView2{}.AddPermissionRequested) / slotSize), wvAddPermissionRequested},
		{"ICoreWebView2::CapturePreview", int(unsafe.Offsetof(vtICoreWebView2{}.CapturePreview) / slotSize), wvCapturePreview},
		{"ICoreWebView2::add_NewWindowRequested", int(unsafe.Offsetof(vtICoreWebView2{}.AddNewWindowRequested) / slotSize), wvAddNewWindowRequested},
		{"ICoreWebView2::RemoveWebResourceRequestedFilter", int(unsafe.Offsetof(vtICoreWebView2{}.RemoveWebResourceRequestedFilter) / slotSize), wvRemoveWebResourceRequestedFilter},
		{"ICoreWebView2::remove_WindowCloseRequested (last)", int(unsafe.Offsetof(vtICoreWebView2{}.RemoveWindowCloseRequested) / slotSize), 60},
		{"ICoreWebView2Controller::get_ZoomFactor", int(unsafe.Offsetof(vtICoreWebView2Controller{}.GetZoomFactor) / slotSize), ctlGetZoomFactor},
		{"ICoreWebView2Controller::put_ZoomFactor", int(unsafe.Offsetof(vtICoreWebView2Controller{}.PutZoomFactor) / slotSize), ctlPutZoomFactor},

		{"ICoreWebView2Settings::put_IsScriptEnabled", setPutIsScriptEnabled, 4},
		{"ICoreWebView2Settings::put_IsWebMessageEnabled", setPutIsWebMessageEnabled, 6},
		{"ICoreWebView2Settings::put_IsStatusBarEnabled", setPutIsStatusBarEnabled, 10},
		{"ICoreWebView2Settings::put_AreDevToolsEnabled", setPutAreDevToolsEnabled, 12},
		{"ICoreWebView2Settings::put_IsBuiltInErrorPageEnabled (last)", int(unsafe.Offsetof(vtICoreWebView2Settings{}.PutIsBuiltInErrorPageEnabled) / slotSize), set2GetUserAgent - 1},

		{"ICoreWebView2NavigationCompletedEventArgs::get_IsSuccess", navCompletedGetIsSuccess, 3},
		{"ICoreWebView2WebMessageReceivedEventArgs::get_Source", msgGetSource, 3},
		{"ICoreWebView2WebMessageReceivedEventArgs::get_WebMessageAsJson", msgGetWebMessageAsJson, 4},
		{"ICoreWebView2WebMessageReceivedEventArgs::TryGetWebMessageAsString", msgTryGetWebMessageAsString, 5},
		{"ICoreWebView2WebResourceRequestedEventArgs::get_Request", resReqGetRequest, 3},
		{"ICoreWebView2WebResourceRequestedEventArgs::put_Response", resReqPutResponse, 5},
		{"ICoreWebView2WebResourceRequest::get_Uri", reqGetURI, 3},
		{"ICoreWebView2WebResourceRequest::get_Method", reqGetMethod, 5},
		{"ICoreWebView2WebResourceRequest::get_Content", reqGetContent, 7},
		{"ICoreWebView2WebResourceRequest::get_Headers", reqGetHeaders, 9},
		{"ICoreWebView2HttpRequestHeaders::GetIterator", headersGetIterator, 8},
		{"ICoreWebView2HttpHeadersCollectionIterator::GetCurrentHeader", iterGetCurrentHeader, 3},
		{"ICoreWebView2HttpHeadersCollectionIterator::get_HasCurrentHeader", iterGetHasCurrentHeader, 4},
		{"ICoreWebView2HttpHeadersCollectionIterator::MoveNext", iterMoveNext, 5},
		{"ICoreWebView2AcceleratorKeyPressedEventArgs::get_KeyEventKind", accelGetKeyEventKind, 3},
		{"ICoreWebView2AcceleratorKeyPressedEventArgs::get_VirtualKey", accelGetVirtualKey, 4},
		{"ICoreWebView2AcceleratorKeyPressedEventArgs::put_Handled", accelPutHandled, 8},

		{"IModalWindow::Show", fdShow, 3},
		{"IFileDialog::SetFileTypes", fdSetFileTypes, 4},
		{"IFileDialog::SetOptions", fdSetOptions, 9},
		{"IFileDialog::GetOptions", fdGetOptions, 10},
		{"IFileDialog::SetFolder", fdSetFolder, 12},
		{"IFileDialog::SetFileName", fdSetFileName, 15},
		{"IFileDialog::SetTitle", fdSetTitle, 17},
		{"IFileDialog::GetResult", fdGetResult, 20},
		{"IFileDialog::SetDefaultExtension", fdSetDefaultExtension, 22},
		{"IFileOpenDialog::GetResults", fodGetResults, 27},
		{"IShellItem::GetDisplayName", siGetDisplayName, 5},
		{"IShellItemArray::GetCount", siaGetCount, 7},
		{"IShellItemArray::GetItemAt", siaGetItemAt, 8},
	} {
		if c.got != c.want {
			t.Errorf("%s: slot %d, want %d", c.name, c.got, c.want)
		}
	}
}

// TestCallbackObject drives a handler through its real vtable, as COM
// would: interface queries, reference counting and Invoke.
func TestCallbackObject(t *testing.T) {
	var gotA, gotB uintptr
	h := newHandler(func(a, b uintptr) { gotA, gotB = a, b })

	if r := comCall(h, unkAddRef); r != 2 {
		t.Errorf("AddRef = %d, want 2", r)
	}
	for i, iid := range cbRefused {
		out := uintptr(1)
		if hr := comCall(h, unkQueryInterface, uintptr(unsafe.Pointer(&iid)), uintptr(unsafe.Pointer(&out))); hr != eNoInterface || out != 0 {
			t.Errorf("refused interface %d: hr %#x, out %#x", i, hr, out)
		}
	}
	iidIUnknown := guid("00000000-0000-0000-C000-000000000046")
	if itf := queryInterface(h, &iidIUnknown); itf != h {
		t.Errorf("QueryInterface(IUnknown) = %#x, want the object %#x", itf, h)
	}
	// Invoke is the first slot after IUnknown, returning S_OK.
	if hr := comCall(h, unkFirstOwn, 7, 9); hr != sOK || gotA != 7 || gotB != 9 {
		t.Errorf("Invoke: hr %#x, args %d %d", hr, gotA, gotB)
	}
	// Three references now (creation, AddRef, QueryInterface).
	for want := uintptr(2); ; want-- {
		if r := comCall(h, unkRelease); r != want {
			t.Fatalf("Release = %d, want %d", r, want)
		}
		if want == 0 {
			break
		}
	}
	if live := liveCallbacks(); live != 0 {
		t.Errorf("%d callback objects live after the last Release", live)
	}

	// Slots are reused, and withHandler drops its creation reference.
	ran := false
	r := withHandler(func(_, _ uintptr) { ran = true }, func(h2 uintptr) uintptr {
		comCall(h2, unkFirstOwn, 0, 0)
		return 42
	})
	if r != 42 || !ran {
		t.Errorf("withHandler = %d, ran %v", r, ran)
	}
	if live := liveCallbacks(); live != 0 {
		t.Errorf("%d callback objects still live", live)
	}
}

func liveCallbacks() int {
	cbMu.Lock()
	defer cbMu.Unlock()
	n := 0
	for _, chunk := range cbChunks {
		for i := range chunk {
			if chunk[i].refs > 0 {
				n++
			}
		}
	}
	return n
}

// TestFailed covers the HRESULT sign test on 64-bit registers.
func TestFailed(t *testing.T) {
	for _, c := range []struct {
		hr   uintptr
		want bool
	}{{0, false}, {1, false}, {0x80004002, true}, {0x800704C7, true}, {0xFFFFFFFF_00000000, false}, {0x7FFFFFFF, false}} {
		if failed(c.hr) != c.want {
			t.Errorf("failed(%#x) = %v", c.hr, !c.want)
		}
	}
}
