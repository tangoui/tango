package platform

// App-owned native views embedded in a canvas surface's layout
// (docs/specs/webview-embed.md, "应用原生视图"). Optional; the core
// discovers support with a type assertion on the Backend.

// NativeViewEmbedder is implemented by backends that can place a native
// view created by the application (an NSView* on macOS, a child HWND on
// Windows, a GtkWidget* on Linux) inside the content area of a canvas
// surface, in the same layer as embedded WebViews.
type NativeViewEmbedder interface {
	// NewEmbeddedNativeView puts view into parent's content area above
	// the canvas; views embedded later stack above earlier ones (WebViews
	// included). It is hidden until a SetPlacement with a non-empty Clip.
	// The application keeps owning view: the backend never destroys it.
	// closed runs on the main thread when the backend removes the view
	// itself because parent closed; it does not run after Close. Main
	// thread.
	NewEmbeddedNativeView(parent SurfaceID, view uintptr, opts NativeViewOptions, closed func()) (EmbeddedNativeView, error)
}

// NativeViewOptions configure an embedded native view.
type NativeViewOptions struct {
	// PassInput lets mouse events over the view reach the canvas below,
	// as if the view were not there; the view never takes keyboard focus
	// from the canvas.
	PassInput bool
}

// EmbeddedNativeView is an application's native view placed in a canvas
// surface.
type EmbeddedNativeView interface {
	// SetPlacement moves, resizes and clips the view, exactly like
	// EmbeddedWebView.SetPlacement. Main thread; cheap when unchanged.
	SetPlacement(p WebPlacement)
	// Close removes the view from its parent and hides it, leaving it to
	// the application. Main thread.
	Close()
}
