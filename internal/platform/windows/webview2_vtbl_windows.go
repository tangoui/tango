//go:build windows && (amd64 || arm64)

package windows

import "unsafe"

// WebView2 interface layouts, transcribed from the method declarations in
// WebView2.h (the Microsoft.Web.WebView2 SDK), in declaration order. Only
// the pointer-sized position of each field matters: a field is one vtable
// slot, and the slot constants below are computed from field offsets, so
// a method is located by naming it rather than by counting.
//
// Later interface versions (ICoreWebView2_2, ICoreWebView2Controller2,
// ...) append to these; the slots here stay valid on every runtime.

// vtIUnknown is the first three slots of every interface.
type vtIUnknown struct {
	QueryInterface, AddRef, Release uintptr
}

type vtICoreWebView2Environment struct {
	vtIUnknown
	CreateCoreWebView2Controller     uintptr
	CreateWebResourceResponse        uintptr
	GetBrowserVersionString          uintptr
	AddNewBrowserVersionAvailable    uintptr
	RemoveNewBrowserVersionAvailable uintptr
}

type vtICoreWebView2Controller struct {
	vtIUnknown
	GetIsVisible                      uintptr
	PutIsVisible                      uintptr
	GetBounds                         uintptr
	PutBounds                         uintptr
	GetZoomFactor                     uintptr
	PutZoomFactor                     uintptr
	AddZoomFactorChanged              uintptr
	RemoveZoomFactorChanged           uintptr
	SetBoundsAndZoomFactor            uintptr
	MoveFocus                         uintptr
	AddMoveFocusRequested             uintptr
	RemoveMoveFocusRequested          uintptr
	AddGotFocus                       uintptr
	RemoveGotFocus                    uintptr
	AddLostFocus                      uintptr
	RemoveLostFocus                   uintptr
	AddAcceleratorKeyPressed          uintptr
	RemoveAcceleratorKeyPressed       uintptr
	GetParentWindow                   uintptr
	PutParentWindow                   uintptr
	NotifyParentWindowPositionChanged uintptr
	Close                             uintptr
	GetCoreWebView2                   uintptr
}

type vtICoreWebView2 struct {
	vtIUnknown
	GetSettings                            uintptr
	GetSource                              uintptr
	Navigate                               uintptr
	NavigateToString                       uintptr
	AddNavigationStarting                  uintptr
	RemoveNavigationStarting               uintptr
	AddContentLoading                      uintptr
	RemoveContentLoading                   uintptr
	AddSourceChanged                       uintptr
	RemoveSourceChanged                    uintptr
	AddHistoryChanged                      uintptr
	RemoveHistoryChanged                   uintptr
	AddNavigationCompleted                 uintptr
	RemoveNavigationCompleted              uintptr
	AddFrameNavigationStarting             uintptr
	RemoveFrameNavigationStarting          uintptr
	AddFrameNavigationCompleted            uintptr
	RemoveFrameNavigationCompleted         uintptr
	AddScriptDialogOpening                 uintptr
	RemoveScriptDialogOpening              uintptr
	AddPermissionRequested                 uintptr
	RemovePermissionRequested              uintptr
	AddProcessFailed                       uintptr
	RemoveProcessFailed                    uintptr
	AddScriptToExecuteOnDocumentCreated    uintptr
	RemoveScriptToExecuteOnDocumentCreated uintptr
	ExecuteScript                          uintptr
	CapturePreview                         uintptr
	Reload                                 uintptr
	PostWebMessageAsJson                   uintptr
	PostWebMessageAsString                 uintptr
	AddWebMessageReceived                  uintptr
	RemoveWebMessageReceived               uintptr
	CallDevToolsProtocolMethod             uintptr
	GetBrowserProcessId                    uintptr
	GetCanGoBack                           uintptr
	GetCanGoForward                        uintptr
	GoBack                                 uintptr
	GoForward                              uintptr
	GetDevToolsProtocolEventReceiver       uintptr
	Stop                                   uintptr
	AddNewWindowRequested                  uintptr
	RemoveNewWindowRequested               uintptr
	AddDocumentTitleChanged                uintptr
	RemoveDocumentTitleChanged             uintptr
	GetDocumentTitle                       uintptr
	AddHostObjectToScript                  uintptr
	RemoveHostObjectFromScript             uintptr
	OpenDevToolsWindow                     uintptr
	AddContainsFullScreenElementChanged    uintptr
	RemoveContainsFullScreenElementChanged uintptr
	GetContainsFullScreenElement           uintptr
	AddWebResourceRequested                uintptr
	RemoveWebResourceRequested             uintptr
	AddWebResourceRequestedFilter          uintptr
	RemoveWebResourceRequestedFilter       uintptr
	AddWindowCloseRequested                uintptr
	RemoveWindowCloseRequested             uintptr
}

type vtICoreWebView2Settings struct {
	vtIUnknown
	GetIsScriptEnabled                uintptr
	PutIsScriptEnabled                uintptr
	GetIsWebMessageEnabled            uintptr
	PutIsWebMessageEnabled            uintptr
	GetAreDefaultScriptDialogsEnabled uintptr
	PutAreDefaultScriptDialogsEnabled uintptr
	GetIsStatusBarEnabled             uintptr
	PutIsStatusBarEnabled             uintptr
	GetAreDevToolsEnabled             uintptr
	PutAreDevToolsEnabled             uintptr
	GetAreDefaultContextMenusEnabled  uintptr
	PutAreDefaultContextMenusEnabled  uintptr
	GetAreHostObjectsAllowed          uintptr
	PutAreHostObjectsAllowed          uintptr
	GetIsZoomControlEnabled           uintptr
	PutIsZoomControlEnabled           uintptr
	GetIsBuiltInErrorPageEnabled      uintptr
	PutIsBuiltInErrorPageEnabled      uintptr
}

type vtICoreWebView2NavigationCompletedEventArgs struct {
	vtIUnknown
	GetIsSuccess      uintptr
	GetWebErrorStatus uintptr
	GetNavigationId   uintptr
}

type vtICoreWebView2WebMessageReceivedEventArgs struct {
	vtIUnknown
	GetSource                uintptr
	GetWebMessageAsJson      uintptr
	TryGetWebMessageAsString uintptr
}

type vtICoreWebView2WebResourceRequestedEventArgs struct {
	vtIUnknown
	GetRequest         uintptr
	GetResponse        uintptr
	PutResponse        uintptr
	GetDeferral        uintptr
	GetResourceContext uintptr
}

type vtICoreWebView2WebResourceRequest struct {
	vtIUnknown
	GetUri     uintptr
	PutUri     uintptr
	GetMethod  uintptr
	PutMethod  uintptr
	GetContent uintptr
	PutContent uintptr
	GetHeaders uintptr
}

type vtICoreWebView2HttpRequestHeaders struct {
	vtIUnknown
	GetHeader    uintptr
	GetHeaders   uintptr
	Contains     uintptr
	SetHeader    uintptr
	RemoveHeader uintptr
	GetIterator  uintptr
}

type vtICoreWebView2HttpHeadersCollectionIterator struct {
	vtIUnknown
	GetCurrentHeader    uintptr
	GetHasCurrentHeader uintptr
	MoveNext            uintptr
}

type vtICoreWebView2AcceleratorKeyPressedEventArgs struct {
	vtIUnknown
	GetKeyEventKind      uintptr
	GetVirtualKey        uintptr
	GetKeyEventLParam    uintptr
	GetPhysicalKeyStatus uintptr
	GetHandled           uintptr
	PutHandled           uintptr
}

// slotSize is the size of one vtable entry; dividing a field offset by it
// gives the slot index comCall takes.
const slotSize = unsafe.Sizeof(uintptr(0))

// Environment, controller and webview slots.
const (
	envCreateController          = int(unsafe.Offsetof(vtICoreWebView2Environment{}.CreateCoreWebView2Controller) / slotSize)
	envCreateWebResourceResponse = int(unsafe.Offsetof(vtICoreWebView2Environment{}.CreateWebResourceResponse) / slotSize)

	ctlPutIsVisible                      = int(unsafe.Offsetof(vtICoreWebView2Controller{}.PutIsVisible) / slotSize)
	ctlPutBounds                         = int(unsafe.Offsetof(vtICoreWebView2Controller{}.PutBounds) / slotSize)
	ctlMoveFocus                         = int(unsafe.Offsetof(vtICoreWebView2Controller{}.MoveFocus) / slotSize)
	ctlAddAcceleratorKeyPressed          = int(unsafe.Offsetof(vtICoreWebView2Controller{}.AddAcceleratorKeyPressed) / slotSize)
	ctlNotifyParentWindowPositionChanged = int(unsafe.Offsetof(vtICoreWebView2Controller{}.NotifyParentWindowPositionChanged) / slotSize)
	ctlClose                             = int(unsafe.Offsetof(vtICoreWebView2Controller{}.Close) / slotSize)
	ctlGetCoreWebView2                   = int(unsafe.Offsetof(vtICoreWebView2Controller{}.GetCoreWebView2) / slotSize)

	wvGetSettings                         = int(unsafe.Offsetof(vtICoreWebView2{}.GetSettings) / slotSize)
	wvGetSource                           = int(unsafe.Offsetof(vtICoreWebView2{}.GetSource) / slotSize)
	wvNavigate                            = int(unsafe.Offsetof(vtICoreWebView2{}.Navigate) / slotSize)
	wvAddNavigationCompleted              = int(unsafe.Offsetof(vtICoreWebView2{}.AddNavigationCompleted) / slotSize)
	wvAddScriptToExecuteOnDocumentCreated = int(unsafe.Offsetof(vtICoreWebView2{}.AddScriptToExecuteOnDocumentCreated) / slotSize)
	wvExecuteScript                       = int(unsafe.Offsetof(vtICoreWebView2{}.ExecuteScript) / slotSize)
	wvAddWebMessageReceived               = int(unsafe.Offsetof(vtICoreWebView2{}.AddWebMessageReceived) / slotSize)
	wvCallDevToolsProtocolMethod          = int(unsafe.Offsetof(vtICoreWebView2{}.CallDevToolsProtocolMethod) / slotSize)
	wvOpenDevToolsWindow                  = int(unsafe.Offsetof(vtICoreWebView2{}.OpenDevToolsWindow) / slotSize)
	wvAddWebResourceRequested             = int(unsafe.Offsetof(vtICoreWebView2{}.AddWebResourceRequested) / slotSize)
	wvAddWebResourceRequestedFilter       = int(unsafe.Offsetof(vtICoreWebView2{}.AddWebResourceRequestedFilter) / slotSize)
)

// Settings and event-argument slots.
const (
	setPutIsScriptEnabled     = int(unsafe.Offsetof(vtICoreWebView2Settings{}.PutIsScriptEnabled) / slotSize)
	setPutIsWebMessageEnabled = int(unsafe.Offsetof(vtICoreWebView2Settings{}.PutIsWebMessageEnabled) / slotSize)
	setPutIsStatusBarEnabled  = int(unsafe.Offsetof(vtICoreWebView2Settings{}.PutIsStatusBarEnabled) / slotSize)
	setPutAreDevToolsEnabled  = int(unsafe.Offsetof(vtICoreWebView2Settings{}.PutAreDevToolsEnabled) / slotSize)

	navCompletedGetIsSuccess = int(unsafe.Offsetof(vtICoreWebView2NavigationCompletedEventArgs{}.GetIsSuccess) / slotSize)

	msgGetSource                = int(unsafe.Offsetof(vtICoreWebView2WebMessageReceivedEventArgs{}.GetSource) / slotSize)
	msgGetWebMessageAsJson      = int(unsafe.Offsetof(vtICoreWebView2WebMessageReceivedEventArgs{}.GetWebMessageAsJson) / slotSize)
	msgTryGetWebMessageAsString = int(unsafe.Offsetof(vtICoreWebView2WebMessageReceivedEventArgs{}.TryGetWebMessageAsString) / slotSize)

	resReqGetRequest  = int(unsafe.Offsetof(vtICoreWebView2WebResourceRequestedEventArgs{}.GetRequest) / slotSize)
	resReqPutResponse = int(unsafe.Offsetof(vtICoreWebView2WebResourceRequestedEventArgs{}.PutResponse) / slotSize)

	reqGetURI     = int(unsafe.Offsetof(vtICoreWebView2WebResourceRequest{}.GetUri) / slotSize)
	reqGetMethod  = int(unsafe.Offsetof(vtICoreWebView2WebResourceRequest{}.GetMethod) / slotSize)
	reqGetContent = int(unsafe.Offsetof(vtICoreWebView2WebResourceRequest{}.GetContent) / slotSize)
	reqGetHeaders = int(unsafe.Offsetof(vtICoreWebView2WebResourceRequest{}.GetHeaders) / slotSize)

	headersGetIterator      = int(unsafe.Offsetof(vtICoreWebView2HttpRequestHeaders{}.GetIterator) / slotSize)
	iterGetCurrentHeader    = int(unsafe.Offsetof(vtICoreWebView2HttpHeadersCollectionIterator{}.GetCurrentHeader) / slotSize)
	iterGetHasCurrentHeader = int(unsafe.Offsetof(vtICoreWebView2HttpHeadersCollectionIterator{}.GetHasCurrentHeader) / slotSize)
	iterMoveNext            = int(unsafe.Offsetof(vtICoreWebView2HttpHeadersCollectionIterator{}.MoveNext) / slotSize)

	accelGetKeyEventKind = int(unsafe.Offsetof(vtICoreWebView2AcceleratorKeyPressedEventArgs{}.GetKeyEventKind) / slotSize)
	accelGetVirtualKey   = int(unsafe.Offsetof(vtICoreWebView2AcceleratorKeyPressedEventArgs{}.GetVirtualKey) / slotSize)
	accelPutHandled      = int(unsafe.Offsetof(vtICoreWebView2AcceleratorKeyPressedEventArgs{}.PutHandled) / slotSize)
)

// WebView2 enumeration values passed to the methods above.
const (
	webResourceContextAll = 0 // COREWEBVIEW2_WEB_RESOURCE_CONTEXT_ALL
	moveFocusProgrammatic = 0 // COREWEBVIEW2_MOVE_FOCUS_REASON_PROGRAMMATIC
	keyEventKeyDown       = 0 // COREWEBVIEW2_KEY_EVENT_KIND_KEY_DOWN
	keyEventSystemKeyDown = 2 // COREWEBVIEW2_KEY_EVENT_KIND_SYSTEM_KEY_DOWN
)
