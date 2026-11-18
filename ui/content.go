package ui

// ContentKind says what a window mostly shows (RunnerOptions.Content). The
// window host uses it to decide when a window leaves its default renderer:
// on Apple Silicon macOS, uihost starts every window, of any size and any
// ContentKind, on native Core Animation layers, whose memory footprint is
// well below Metal's, and switches a window to Metal at most once.
// TANGO_RENDERER and RunnerOptions.Renderer still take precedence, and hosts
// without such a choice ignore it.
type ContentKind int

const (
	// ContentGeneral is the default. On macOS the window switches itself
	// to Metal on the frame after its first keyboard input (a non-modifier
	// key press, committed text or an IME composition), because Metal shows
	// input about two frames sooner, or once its frames stay over 8 ms.
	// Scrolling and pointer input alone keep it on native layers.
	ContentGeneral ContentKind = iota
	// ContentData is for windows that mostly display data changing on its
	// own: tables, grids, quote boards, lists and dashboards. On macOS they
	// ignore keyboard input, so browsing with arrow or page keys keeps their
	// memory footprint low (list and quote windows: 39-54 vs 135-147 MB on
	// Metal; an 800x600 grid redrawing 600 cells every frame: about 69 MB),
	// and switch to Metal only once their frames stay over 8 ms. Their input
	// reaches the screen about two frames later than on Metal, so editors,
	// terminals and drawing canvases should stay ContentGeneral.
	ContentData
)
