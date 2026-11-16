//go:build darwin

package macoslite

// App owns one native window. Create it with New and call Run on the main
// goroutine.
type App struct {
	opts Options
	view View
}

// New validates the options and returns an App. It does not touch native
// APIs; the window is created by Run.
func New(o Options, v View) (*App, error) {
	if err := validate(o, v); err != nil {
		return nil, err
	}
	return &App{opts: o, view: v}, nil
}

// Run creates the window and runs the AppKit event loop until Stop or the
// window closes. It must be called on the main goroutine.
func (a *App) Run() error { return ErrUnsupported }

// Stop closes the window and ends Run. Safe to call more than once.
func (a *App) Stop() {}

// Invalidate schedules a redraw. Safe from any goroutine.
func (a *App) Invalidate() {}

// TrimCaches releases cached fonts; the current frame stays on screen.
func (a *App) TrimCaches() {}

// Canvas draws into the current frame's bitmap.
type Canvas struct{}

func (*Canvas) Fill(Rect, Color)                   {}
func (*Canvas) RoundRect(Rect, float64, Color)     {}
func (*Canvas) Text(string, Point, float64, Color) {}
