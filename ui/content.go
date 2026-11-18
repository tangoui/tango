package ui

// ContentKind says what a window mostly shows (RunnerOptions.Content). The
// window host uses it to pick a default renderer: on macOS, uihost draws
// ContentData windows with native layers at every size. TANGO_RENDERER and
// RunnerOptions.Renderer still take precedence, and hosts without such a
// choice ignore it.
type ContentKind int

const (
	// ContentGeneral is the default. On macOS small windows draw with
	// native Core Animation layers and large ones with Metal, which shows
	// typing, scrolling and dragging about two frames sooner.
	ContentGeneral ContentKind = iota
	// ContentData is for windows that mostly display data changing on its
	// own: tables, grids, quote boards, lists and dashboards. On macOS they
	// draw with native layers at every size, which keeps memory footprint
	// well below Metal's (list and quote windows: 39-54 vs 135-147 MB; an
	// 800x600 grid redrawing 600 cells every frame: about 69 MB). A window
	// whose frames stay over 8 ms still switches itself to Metal once.
	// Input reaches the screen about two frames later than on Metal, so
	// editors, terminals and drawing canvases should stay ContentGeneral.
	ContentData
)
