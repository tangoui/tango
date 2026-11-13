//go:build windows && (amd64 || arm64)

package windows

// UI Automation for canvases (docs/specs/a11y-bridge.md,
// docs/accessibility.md): the canvas child window answers WM_GETOBJECT
// with a provider tree built from the a11y.Tree the core hands to
// platform.CanvasAccessible. Constants, vtable layouts, property values
// and navigation are in uia_plan.go.
//
// Objects. Every exposed node is one Go-allocated COM object (uiaObj)
// with several interface pointers, one per implemented interface
// (IRawElementProviderSimple, IRawElementProviderFragment, the root's
// IRawElementProviderFragmentRoot and the control patterns). An interface
// pointer is the address of one vtable slot of the object's itf array;
// uiaByItf turns it back into (object, interface), so a stale or foreign
// pointer is detected instead of dereferenced. Objects are cached per
// NodeID while the node exists (the cache holds one reference); a node
// that disappears is marked stale (its methods return
// UIA_E_ELEMENTNOTAVAILABLE) and disconnected.
//
// Threading. The providers return ProviderOptions_UseComThreading, so UIA
// calls them through COM on the canvas's (STA main) thread. Only AddRef,
// Release and QueryInterface are written for any thread. Actions go to
// the sink directly when the call arrives on the main thread outside a
// sink callback, otherwise they are queued and run from a posted message.
//
// Activation. The first WM_GETOBJECT for UiaRootObjectId (or OBJID_CLIENT,
// served by UIA's MSAA bridge) makes AccessibilityActive true and asks for
// a frame; the core then delivers the full tree. There is no deactivation.

import (
	"math"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

var _ platform.CanvasAccessible = (*canvas)(nil)

const (
	canvasWmGetObject = 0x003D
	wmCanvasA11y      = 0x0400 + 0x7A4 // canvas class private: run queued accessibility actions
	objidClient       = -4
)

var (
	uiaCore  = systemDLL("UIAutomationCore.dll")
	oleAut32 = systemDLL("oleaut32.dll")

	uiaReturnRawElementProvider   = uiaCore.NewProc("UiaReturnRawElementProvider")
	uiaHostProviderFromHwnd       = uiaCore.NewProc("UiaHostProviderFromHwnd")
	uiaClientsAreListening        = uiaCore.NewProc("UiaClientsAreListening")
	uiaRaiseAutomationEvent       = uiaCore.NewProc("UiaRaiseAutomationEvent")
	uiaRaisePropertyChangedEvent  = uiaCore.NewProc("UiaRaiseAutomationPropertyChangedEvent")
	uiaRaiseStructureChangedEvent = uiaCore.NewProc("UiaRaiseStructureChangedEvent")
	uiaRaiseNotificationEvent     = uiaCore.NewProc("UiaRaiseNotificationEvent") // Windows 10 1709+
	uiaDisconnectProvider         = uiaCore.NewProc("UiaDisconnectProvider")     // Windows 8+

	sysAllocString        = oleAut32.NewProc("SysAllocString")
	sysFreeString         = oleAut32.NewProc("SysFreeString")
	safeArrayCreateVector = oleAut32.NewProc("SafeArrayCreateVector")
	safeArrayAccessData   = oleAut32.NewProc("SafeArrayAccessData")
	safeArrayUnaccessData = oleAut32.NewProc("SafeArrayUnaccessData")

	a11yGetCurrentThreadID   = kernel32.export("GetCurrentThreadId")
	a11yGetCurrentProcess    = kernel32.export("GetCurrentProcess")
	a11yVirtualAlloc         = kernel32.export("VirtualAlloc")
	a11yVirtualProtect       = kernel32.export("VirtualProtect")
	a11yFlushInstructionCach = kernel32.export("FlushInstructionCache")
)

// variant is VARIANT (24 bytes on 64-bit Windows).
type variant struct {
	VT  uint16
	_   [3]uint16
	Val uint64
	_   uint64
}

const (
	vtEmpty = 0
	vtI4    = 3
	vtR8    = 5
	vtBSTR  = 8
	vtBool  = 11
)

// uiaRect is UiaRect.
type uiaRect struct{ Left, Top, Width, Height float64 }

var (
	_ = [1]struct{}{}[unsafe.Sizeof(variant{})-24]
	_ = [1]struct{}{}[unsafe.Sizeof(uiaRect{})-32]
)

// Interfaces of a uiaObj, indexes into its itf array.
const (
	itfSimple = iota
	itfFrag
	itfRoot
	itfInvoke
	itfToggle
	itfValue
	itfRange
	itfExpand
	itfSel
	itfScroll
	itfCount
)

// uiaObj is one provider object.
type uiaObj struct {
	itf   [itfCount]*uintptr // interface pointers: &itf[k] is what COM sees
	refs  atomic.Int32
	ax    *canvasA11y
	id    a11y.NodeID // node; the root object resolves tree.Root
	root  bool
	stale atomic.Bool
}

type uiaRef struct {
	o *uiaObj
	k int
}

var (
	uiaMu    sync.Mutex
	uiaByItf = map[uintptr]uiaRef{} // interface pointer → object

	uiaIIDs   [itfCount]GUID
	iidUnk    GUID
	uiaVtOnce sync.Once
	uiaVt     struct {
		simple [uiaSimpleSlots]uintptr
		frag   [uiaFragSlots]uintptr
		root   [uiaRootSlots]uintptr
		invoke [uiaInvokeSlots]uintptr
		toggle [uiaToggleSlots]uintptr
		value  [uiaValueSlots]uintptr
		rng    [uiaRangeSlots]uintptr
		expand [uiaExpandSlots]uintptr
		sel    [uiaSelSlots]uintptr
		scroll [uiaScrollItemSlots]uintptr
	}
	uiaVtErr error
)

// canvasA11y is the accessibility state of one canvas. Main thread unless
// noted.
type canvasA11y struct {
	c      *canvas
	active atomic.Bool
	tree   atomic.Pointer[a11y.Tree]
	focus  a11y.NodeID
	tid    uintptr // the canvas's thread
	root   *uiaObj
	objs   map[a11y.NodeID]*uiaObj

	mu      sync.Mutex
	pending []pendingAction // actions waiting for wmCanvasA11y
	posted  bool
}

type pendingAction struct {
	id  a11y.NodeID
	act a11y.Action
	arg string
}

// ---- platform.CanvasAccessible ----

// AccessibilityActive implements platform.CanvasAccessible. Any goroutine.
func (c *canvas) AccessibilityActive() bool {
	ax := c.ax
	return ax != nil && ax.active.Load()
}

// SetAccessibilityTree implements platform.CanvasAccessible.
func (c *canvas) SetAccessibilityTree(t *a11y.Tree) {
	ax := c.ax
	if ax == nil || c.closed {
		return
	}
	ax.tree.Store(t)
	for id, o := range ax.objs {
		if t.Node(id) == nil {
			ax.drop(id, o)
		}
	}
	if t == nil {
		ax.focus = 0
	}
	if listening() {
		uiaRaiseStructureChangedEvent.Call(ax.root.ptr(itfSimple), uiaStructChildrenInvalidated, 0, 0)
	}
}

// UpdateAccessibilityNodes implements platform.CanvasAccessible.
func (c *canvas) UpdateAccessibilityNodes(t *a11y.Tree, changed []a11y.NodeID) {
	ax := c.ax
	if ax == nil || c.closed {
		return
	}
	prev := ax.tree.Swap(t)
	listen := listening()
	for _, id := range changed {
		pn, nn := prev.Node(id), t.Node(id)
		if nn == nil {
			if o := ax.objs[id]; o != nil {
				ax.drop(id, o)
			}
			continue
		}
		if !listen || pn == nil {
			continue
		}
		ch := a11ybridge.Diff(pn, nn)
		o := ax.objs[id]
		if id == t.Root {
			o = ax.root
		}
		if o == nil {
			continue // never handed out: no client knows it
		}
		if ch.Has(a11ybridge.ChangeChildren) {
			rid := uiaRuntimeID(id)
			p, n := uintptr(unsafe.Pointer(&rid[0])), uintptr(len(rid))
			if o.root {
				p, n = 0, 0
			}
			uiaRaiseStructureChangedEvent.Call(o.ptr(itfSimple), uiaStructChildrenInvalidated, p, n)
			runtime.KeepAlive(&rid)
		}
		for _, prop := range uiaPropertyEvents(ch, nn) {
			ov := uiaProperty(prev, pn, prop, c.hasFocus, o.root)
			nv := uiaProperty(t, nn, prop, c.hasFocus, o.root)
			if ov == nv {
				continue
			}
			oldV, newV := toVariant(ov), toVariant(nv)
			uiaRaisePropertyChangedEvent.Call(o.ptr(itfSimple), uintptr(prop),
				uintptr(unsafe.Pointer(&oldV)), uintptr(unsafe.Pointer(&newV)))
			clearVariant(&oldV)
			clearVariant(&newV)
		}
	}
}

// AccessibilityFocusChanged implements platform.CanvasAccessible.
func (c *canvas) AccessibilityFocusChanged(id a11y.NodeID) {
	ax := c.ax
	if ax == nil || c.closed {
		return
	}
	ax.focus = id
	if id == 0 || !c.hasFocus || !listening() {
		return
	}
	if o := ax.obj(id); o != nil {
		uiaRaiseAutomationEvent.Call(o.ptr(itfSimple), uiaAutomationFocusChangedEventID)
	}
}

// AccessibilityAnnounce implements platform.CanvasAccessible.
func (c *canvas) AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID) {
	ax := c.ax
	if ax == nil || c.closed || text == "" || !listening() {
		return
	}
	if t := ax.tree.Load(); source != 0 && t.Node(source) != nil && t.Node(source).Live != a11y.LiveOff {
		if o := ax.obj(source); o != nil {
			uiaRaiseAutomationEvent.Call(o.ptr(itfSimple), uiaLiveRegionChangedEventID)
			return
		}
	}
	if !has(uiaRaiseNotificationEvent) {
		return // before Windows 10 1709 there is no way to speak free text
	}
	msg := bstr(text)
	act := bstr("tango.announce")
	uiaRaiseNotificationEvent.Call(ax.root.ptr(itfSimple), uiaNotificationOther, uintptr(uiaProcessing(live)), msg, act)
	sysFreeString.Call(msg)
	sysFreeString.Call(act)
}

func listening() bool {
	if !has(uiaClientsAreListening) {
		return false
	}
	r, _, _ := uiaClientsAreListening.Call()
	return r != 0
}

// ---- canvas hooks (canvas_windows.go) ----

// a11yMessage handles WM_GETOBJECT and wmCanvasA11y.
func (c *canvas) a11yMessage(m uint32, wp, lp uintptr) (uintptr, bool) {
	if c.closed {
		return 0, false
	}
	if m == wmCanvasA11y {
		if c.ax != nil {
			c.ax.runPending()
		}
		return 0, true
	}
	obj := int32(lp)
	if obj != uiaRootObjectID && obj != objidClient {
		return 0, false
	}
	if !has(uiaReturnRawElementProvider) {
		return 0, false
	}
	ax := c.ax
	if ax == nil {
		if err := uiaInitVtables(); err != nil {
			return 0, false
		}
		tid, _, _ := a11yGetCurrentThreadID.Call()
		ax = &canvasA11y{c: c, tid: tid, objs: map[a11y.NodeID]*uiaObj{}}
		ax.root = ax.newObj(0, true)
		c.ax = ax
	}
	if !ax.active.Swap(true) {
		c.Invalidate() // the next frame brings the first tree
	}
	r, _, _ := uiaReturnRawElementProvider.Call(c.hwnd, wp, lp, ax.root.ptr(itfSimple))
	return r, true
}

// a11yDestroy disconnects every provider; called from destroy.
func (c *canvas) a11yDestroy() {
	ax := c.ax
	if ax == nil {
		return
	}
	ax.tree.Store(nil)
	for id, o := range ax.objs {
		ax.drop(id, o)
	}
	if has(uiaReturnRawElementProvider) {
		uiaReturnRawElementProvider.Call(c.hwnd, 0, 0, 0)
	}
	ax.drop(0, ax.root)
	ax.active.Store(false)
}

// ---- objects ----

func (ax *canvasA11y) newObj(id a11y.NodeID, root bool) *uiaObj {
	o := &uiaObj{ax: ax, id: id, root: root}
	o.itf = [itfCount]*uintptr{
		&uiaVt.simple[0], &uiaVt.frag[0], &uiaVt.root[0], &uiaVt.invoke[0], &uiaVt.toggle[0],
		&uiaVt.value[0], &uiaVt.rng[0], &uiaVt.expand[0], &uiaVt.sel[0], &uiaVt.scroll[0],
	}
	o.refs.Store(1) // the cache's reference
	uiaMu.Lock()
	for k := range o.itf {
		uiaByItf[o.ptr(k)] = uiaRef{o, k}
	}
	uiaMu.Unlock()
	return o
}

// ptr is the interface pointer of interface k.
func (o *uiaObj) ptr(k int) uintptr { return uintptr(unsafe.Pointer(&o.itf[k])) }

// obj returns the cached object of node id (the root object for
// tree.Root), creating it; nil when id is not in the tree.
func (ax *canvasA11y) obj(id a11y.NodeID) *uiaObj {
	t := ax.tree.Load()
	if t.Node(id) == nil {
		return nil
	}
	if id == t.Root {
		return ax.root
	}
	o := ax.objs[id]
	if o == nil {
		o = ax.newObj(id, false)
		ax.objs[id] = o
	}
	return o
}

// drop marks o stale, disconnects it from clients and releases the
// cache's reference.
func (ax *canvasA11y) drop(id a11y.NodeID, o *uiaObj) {
	if o.stale.Swap(true) {
		return
	}
	if !o.root {
		delete(ax.objs, id)
	}
	if has(uiaDisconnectProvider) {
		uiaDisconnectProvider.Call(o.ptr(itfSimple))
	}
	o.release()
}

func (o *uiaObj) addRef() int32 { return o.refs.Add(1) }

func (o *uiaObj) release() int32 {
	n := o.refs.Add(-1)
	if n <= 0 {
		uiaMu.Lock()
		for k := range o.itf {
			delete(uiaByItf, o.ptr(k))
		}
		uiaMu.Unlock()
	}
	return n
}

// node returns o's node in the current tree (nil when stale or gone).
func (o *uiaObj) node() (*a11y.Tree, *a11y.Node) {
	if o.stale.Load() {
		return nil, nil
	}
	t := o.ax.tree.Load()
	if t == nil {
		return nil, nil
	}
	if o.root {
		return t, t.Node(t.Root)
	}
	return t, t.Node(o.id)
}

func lookup(this uintptr) (*uiaObj, int) {
	uiaMu.Lock()
	r, ok := uiaByItf[this]
	uiaMu.Unlock()
	if !ok {
		return nil, -1
	}
	return r.o, r.k
}

// supports reports whether o answers QueryInterface for interface k.
func (o *uiaObj) supports(k int) bool {
	switch k {
	case itfSimple, itfFrag:
		return true
	case itfRoot:
		return o.root
	}
	_, n := o.node()
	if n == nil {
		return false
	}
	return uiaHasPattern(n, itfPattern[k])
}

// itfPattern maps pattern interfaces to their pattern IDs.
var itfPattern = [itfCount]int32{
	itfInvoke: a11ybridge.UIAInvokePattern, itfToggle: a11ybridge.UIATogglePattern,
	itfValue: a11ybridge.UIAValuePattern, itfRange: a11ybridge.UIARangeValuePattern,
	itfExpand: a11ybridge.UIAExpandCollapsePattern, itfSel: a11ybridge.UIASelectionItemPattern,
	itfScroll: a11ybridge.UIAScrollItemPattern,
}

// ---- actions ----

// perform runs act on node id for a UIA client.
func (ax *canvasA11y) perform(n *a11y.Node, act a11y.Action, arg string) uintptr {
	if act == 0 || !n.Actions.Has(act) {
		return uiaEInvalidOperation
	}
	c := ax.c
	if c.closed || !c.ready {
		return uiaEElementNotAvailable
	}
	sink, ok := c.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return uiaENotSupported
	}
	tid, _, _ := a11yGetCurrentThreadID.Call()
	if tid == ax.tid && c.sinkDepth == 0 && !c.inPaint {
		if act == a11y.Focus {
			canvasSetFocus.Call(c.hwnd)
		}
		c.sinkDepth++
		err := sink.AccessibilityAction(n.ID, act, arg)
		c.sinkDepth--
		if err != nil {
			return uiaEInvalidOperation
		}
		return sOK
	}
	ax.mu.Lock()
	ax.pending = append(ax.pending, pendingAction{n.ID, act, arg})
	post := !ax.posted
	ax.posted = true
	ax.mu.Unlock()
	if post {
		postMessage(c.hwnd, wmCanvasA11y, 0, 0)
	}
	return sOK
}

func (ax *canvasA11y) runPending() {
	ax.mu.Lock()
	list := ax.pending
	ax.pending, ax.posted = nil, false
	ax.mu.Unlock()
	t := ax.tree.Load()
	for _, p := range list {
		if n := t.Node(p.id); n != nil {
			ax.perform(n, p.act, p.arg)
		}
	}
}

// ---- values ----

func bstr(s string) uintptr {
	p, _, _ := sysAllocString.Call(uintptr(unsafe.Pointer(u16(s))))
	return p
}

func toVariant(v uiaValue) variant {
	var out variant
	switch v.Kind {
	case uiaI4:
		out.VT, out.Val = vtI4, uint64(uint32(v.I))
	case uiaBool:
		out.VT = vtBool
		if v.B {
			out.Val = 0xFFFF // VARIANT_TRUE
		}
	case uiaStr:
		out.VT, out.Val = vtBSTR, uint64(bstr(v.S))
	case uiaR8:
		out.VT, out.Val = vtR8, math.Float64bits(v.F)
	}
	return out
}

func clearVariant(v *variant) {
	if v.VT == vtBSTR && v.Val != 0 {
		sysFreeString.Call(uintptr(v.Val))
	}
	*v = variant{}
}

// ---- vtables ----

func uiaInitVtables() error {
	uiaVtOnce.Do(func() {
		for k, s := range [itfCount]string{
			iidIRawElementProviderSimple, iidIRawElementProviderFragment, iidIRawElementProviderFragmentRoot,
			iidIInvokeProvider, iidIToggleProvider, iidIValueProvider, iidIRangeValueProvider,
			iidIExpandCollapseProvider, iidISelectionItemProvider, iidIScrollItemProvider,
		} {
			uiaIIDs[k] = guid(s)
		}
		iidUnk = guid(iidIUnknown)
		fromPoint, err := makeThunk(thunkFromPoint(runtime.GOARCH, uint64(syscall.NewCallback(uiaFromPoint))))
		if err != nil {
			uiaVtErr = err
			return
		}
		setRange, err := makeThunk(thunkOneDouble(runtime.GOARCH, uint64(syscall.NewCallback(uiaRangeSet))))
		if err != nil {
			uiaVtErr = err
			return
		}
		unk := [3]uintptr{syscall.NewCallback(uiaQI), syscall.NewCallback(uiaAddRef), syscall.NewCallback(uiaRelease)}
		fill := func(vt []uintptr, own ...uintptr) {
			copy(vt, unk[:])
			copy(vt[3:], own)
		}
		cb := syscall.NewCallback
		fill(uiaVt.simple[:], cb(uiaProviderOptions), cb(uiaPatternProvider), cb(uiaPropertyValue), cb(uiaHostProvider))
		fill(uiaVt.frag[:], cb(uiaNavigateCB), cb(uiaRuntimeIDCB), cb(uiaBounds), cb(uiaEmbedded), cb(uiaSetFocus), cb(uiaFragmentRoot))
		fill(uiaVt.root[:], fromPoint, cb(uiaGetFocus))
		fill(uiaVt.invoke[:], cb(uiaInvoke))
		fill(uiaVt.toggle[:], cb(uiaToggle), cb(uiaToggleState))
		fill(uiaVt.value[:], cb(uiaValueSet), cb(uiaValueGet), cb(uiaValueReadOnly))
		fill(uiaVt.rng[:], setRange, cb(uiaRangeValue), cb(uiaRangeReadOnly), cb(uiaRangeMax), cb(uiaRangeMin), cb(uiaRangeStep), cb(uiaRangeStep))
		fill(uiaVt.expand[:], cb(uiaExpand), cb(uiaCollapse), cb(uiaExpandState))
		fill(uiaVt.sel[:], cb(uiaSelect), cb(uiaAddToSel), cb(uiaRemoveFromSel), cb(uiaIsSelected), cb(uiaSelContainer))
		fill(uiaVt.scroll[:], cb(uiaScrollIntoView))
	})
	return uiaVtErr
}

// makeThunk copies code into executable memory and returns its address.
func makeThunk(code []byte) (uintptr, error) {
	if len(code) == 0 {
		return 0, errUnsupportedArch
	}
	const memCommitReserve, pageRW, pageRX = 0x3000, 0x04, 0x20
	mem, _, err := a11yVirtualAlloc.Call(0, uintptr(len(code)), memCommitReserve, pageRW)
	if mem == 0 {
		return 0, err
	}
	copy(unsafe.Slice((*byte)(native(mem)), len(code)), code)
	var old uint32
	if r, _, err := a11yVirtualProtect.Call(mem, uintptr(len(code)), pageRX, uintptr(unsafe.Pointer(&old))); r == 0 {
		return 0, err
	}
	proc, _, _ := a11yGetCurrentProcess.Call()
	a11yFlushInstructionCach.Call(proc, mem, uintptr(len(code)))
	return mem, nil
}

var errUnsupportedArch = syscall.Errno(50) // ERROR_NOT_SUPPORTED

func outPtr(p uintptr, v uintptr) { *(*uintptr)(native(p)) = v }
func outI32(p uintptr, v int32)   { *(*int32)(native(p)) = v }
func outF64(p uintptr, v float64) { *(*float64)(native(p)) = v }
func outBool(p uintptr, b bool) {
	var v int32
	if b {
		v = 1
	}
	*(*int32)(native(p)) = v
}

// self resolves this to its object; stale objects report not available.
func self(this uintptr) (*uiaObj, *a11y.Tree, *a11y.Node, uintptr) {
	o, _ := lookup(this)
	if o == nil {
		return nil, nil, nil, uiaEElementNotAvailable
	}
	t, n := o.node()
	if n == nil && !(o.root && !o.stale.Load()) {
		return o, nil, nil, uiaEElementNotAvailable
	}
	return o, t, n, sOK
}

// handOut returns the interface pointer k of node id with a reference for
// the caller (0 when id is 0 or not in the tree).
func handOut(ax *canvasA11y, id a11y.NodeID, k int) uintptr {
	if id == 0 {
		return 0
	}
	o := ax.obj(id)
	if o == nil {
		return 0
	}
	o.addRef()
	return o.ptr(k)
}

// ---- IUnknown ----

func uiaQI(this, riid, out uintptr) uintptr {
	if out == 0 {
		return ePointer
	}
	outPtr(out, 0)
	o, _ := lookup(this)
	if o == nil {
		return eNoInterface
	}
	want := *(*GUID)(native(riid))
	k := -1
	if want == iidUnk {
		k = itfSimple
	} else {
		for i, g := range uiaIIDs {
			if g == want {
				k = i
				break
			}
		}
	}
	if k < 0 || !o.supports(k) {
		return eNoInterface
	}
	o.addRef()
	outPtr(out, o.ptr(k))
	return sOK
}

func uiaAddRef(this uintptr) uintptr {
	if o, _ := lookup(this); o != nil {
		return uintptr(o.addRef())
	}
	return 0
}

func uiaRelease(this uintptr) uintptr {
	if o, _ := lookup(this); o != nil {
		return uintptr(max(o.release(), 0))
	}
	return 0
}

// ---- IRawElementProviderSimple ----

func uiaProviderOptions(this, out uintptr) uintptr {
	outI32(out, uiaProviderServerSide|uiaProviderUseComThreading)
	return sOK
}

func uiaPatternProvider(this, pattern, out uintptr) uintptr {
	outPtr(out, 0)
	o, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	for k := itfInvoke; k < itfCount; k++ {
		if itfPattern[k] == int32(pattern) && uiaHasPattern(n, int32(pattern)) {
			o.addRef()
			outPtr(out, o.ptr(k))
			break
		}
	}
	return sOK
}

func uiaPropertyValue(this, prop, out uintptr) uintptr {
	v := (*variant)(native(out))
	*v = variant{}
	o, t, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	*v = toVariant(uiaProperty(t, n, int32(prop), o.ax.c.hasFocus, o.root))
	return sOK
}

func uiaHostProvider(this, out uintptr) uintptr {
	outPtr(out, 0)
	o, _ := lookup(this)
	if o == nil || !o.root || o.stale.Load() {
		return sOK
	}
	hr, _, _ := uiaHostProviderFromHwnd.Call(o.ax.c.hwnd, out)
	return hr
}

// ---- IRawElementProviderFragment ----

func uiaNavigateCB(this, dir, out uintptr) uintptr {
	outPtr(out, 0)
	o, t, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	outPtr(out, handOut(o.ax, uiaNavigate(t, n.ID, int32(dir)), itfFrag))
	return sOK
}

func uiaRuntimeIDCB(this, out uintptr) uintptr {
	outPtr(out, 0)
	o, _, n, hr := self(this)
	if hr != sOK || o.root || n == nil {
		return hr // the root's runtime ID comes from its host window
	}
	rid := uiaRuntimeID(n.ID)
	sa, _, _ := safeArrayCreateVector.Call(vtI4, 0, uintptr(len(rid)))
	if sa == 0 {
		return eFail
	}
	var data uintptr
	if r, _, _ := safeArrayAccessData.Call(sa, uintptr(unsafe.Pointer(&data))); failed(r) {
		return r
	}
	copy(unsafe.Slice((*int32)(native(data)), len(rid)), rid[:])
	safeArrayUnaccessData.Call(sa)
	outPtr(out, sa)
	return sOK
}

func uiaBounds(this, out uintptr) uintptr {
	r := (*uiaRect)(native(out))
	*r = uiaRect{}
	o, _, n, hr := self(this)
	if hr != sOK {
		return hr
	}
	c := o.ax.c
	var b a11y.Rect
	if n != nil && !(o.root && n.Bounds.W == 0) {
		b = n.Bounds
	} else {
		b = a11y.Rect{W: float32(c.w), H: float32(c.h)}
	}
	var org point
	canvasClientToScreen.Call(c.hwnd, uintptr(unsafe.Pointer(&org)))
	d := a11ybridge.ToDevice(b, c.scale, org.X, org.Y)
	*r = uiaRect{float64(d.X), float64(d.Y), float64(d.W), float64(d.H)}
	return sOK
}

func uiaEmbedded(this, out uintptr) uintptr { outPtr(out, 0); return sOK }

func uiaSetFocus(this uintptr) uintptr {
	o, _, n, hr := self(this)
	if hr != sOK {
		return hr
	}
	if n == nil || o.root || !n.Actions.Has(a11y.Focus) {
		canvasSetFocus.Call(o.ax.c.hwnd)
		return sOK
	}
	return o.ax.perform(n, a11y.Focus, "")
}

func uiaFragmentRoot(this, out uintptr) uintptr {
	outPtr(out, 0)
	o, _ := lookup(this)
	if o == nil || o.stale.Load() {
		return uiaEElementNotAvailable
	}
	o.ax.root.addRef()
	outPtr(out, o.ax.root.ptr(itfRoot))
	return sOK
}

// ---- IRawElementProviderFragmentRoot ----

// uiaFromPoint is ElementProviderFromPoint behind the thunk: x and y
// arrive as the bits of the doubles (screen pixels).
func uiaFromPoint(this, xbits, ybits, out uintptr) uintptr {
	outPtr(out, 0)
	o, t, _, hr := self(this)
	if hr != sOK || t == nil {
		return hr
	}
	c := o.ax.c
	var org point
	canvasClientToScreen.Call(c.hwnd, uintptr(unsafe.Pointer(&org)))
	x, y := a11ybridge.FromDevice(int32(math.Floor(f64(xbits))), int32(math.Floor(f64(ybits))), c.scale, org.X, org.Y)
	id := a11ybridge.HitTest(t, x, y)
	if id == 0 {
		return sOK
	}
	outPtr(out, handOut(o.ax, id, itfFrag))
	return sOK
}

func uiaGetFocus(this, out uintptr) uintptr {
	outPtr(out, 0)
	o, t, _, hr := self(this)
	if hr != sOK || t == nil || !o.ax.c.hasFocus {
		return hr
	}
	if f := a11ybridge.EffectiveFocus(t); f != 0 && f != t.Root {
		outPtr(out, handOut(o.ax, f, itfFrag))
	}
	return sOK
}

// ---- patterns ----

func act(this uintptr, a a11y.Action, arg string) uintptr {
	o, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	return o.ax.perform(n, a, arg)
}

func uiaInvoke(this uintptr) uintptr { return act(this, a11y.Press, "") }
func uiaToggle(this uintptr) uintptr { return act(this, a11y.Press, "") }

func uiaToggleState(this, out uintptr) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	outI32(out, a11ybridge.ToggleState(n))
	return sOK
}

func uiaValueSet(this, text uintptr) uintptr { return act(this, a11y.SetValue, wstr(text)) }

func uiaValueGet(this, out uintptr) uintptr {
	outPtr(out, 0)
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	outPtr(out, bstr(uiaValueText(n)))
	return sOK
}

func uiaValueReadOnly(this, out uintptr) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	outBool(out, n.States.Has(a11y.ReadOnly) || !n.Actions.Has(a11y.SetValue))
	return sOK
}

// uiaRangeSet is IRangeValueProvider::SetValue behind the thunk.
func uiaRangeSet(this, bits uintptr) uintptr {
	o, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	v := f64(bits)
	if n.HasRange && (v < n.ValueMin || v > n.ValueMax) {
		return eInvalidArg
	}
	a := a11ybridge.RangeAction(n, v)
	if a == 0 {
		return sOK
	}
	return o.ax.perform(n, a, strconv.FormatFloat(v, 'g', -1, 64))
}

func rangeGet(this, out uintptr, get func(n *a11y.Node) float64) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	outF64(out, get(n))
	return sOK
}

func uiaRangeValue(this, out uintptr) uintptr {
	return rangeGet(this, out, func(n *a11y.Node) float64 { return n.ValueNow })
}
func uiaRangeMax(this, out uintptr) uintptr {
	return rangeGet(this, out, func(n *a11y.Node) float64 { return n.ValueMax })
}
func uiaRangeMin(this, out uintptr) uintptr {
	return rangeGet(this, out, func(n *a11y.Node) float64 { return n.ValueMin })
}
func uiaRangeStep(this, out uintptr) uintptr { return rangeGet(this, out, a11ybridge.RangeStep) }

func uiaRangeReadOnly(this, out uintptr) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	ro := n.States.Has(a11y.ReadOnly) || n.Actions&(a11y.SetValue|a11y.Increment|a11y.Decrement) == 0
	outBool(out, ro)
	return sOK
}

func uiaExpand(this uintptr) uintptr   { return act(this, a11y.Expand, "") }
func uiaCollapse(this uintptr) uintptr { return act(this, a11y.Collapse, "") }

func uiaExpandState(this, out uintptr) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	outI32(out, a11ybridge.ExpandCollapseState(n))
	return sOK
}

func uiaSelect(this uintptr) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	if n.States.Has(a11y.Selected) {
		return sOK
	}
	return act(this, a11y.Press, "")
}

func uiaAddToSel(this uintptr) uintptr { return uiaSelect(this) }

func uiaRemoveFromSel(this uintptr) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	if !n.States.Has(a11y.Selected) {
		return sOK
	}
	return act(this, a11y.Press, "")
}

func uiaIsSelected(this, out uintptr) uintptr {
	_, _, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	outBool(out, n.States.Has(a11y.Selected) || (n.Role == a11y.RoleRadio && n.States.Has(a11y.Checked)))
	return sOK
}

func uiaSelContainer(this, out uintptr) uintptr {
	outPtr(out, 0)
	o, t, n, hr := self(this)
	if hr != sOK || n == nil {
		return hr
	}
	if id := uiaSelectionContainer(t, n.ID); id != 0 {
		outPtr(out, handOut(o.ax, id, itfSimple))
	}
	return sOK
}

func uiaScrollIntoView(this uintptr) uintptr { return act(this, a11y.ScrollIntoView, "") }
