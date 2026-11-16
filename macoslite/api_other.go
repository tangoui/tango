//go:build !darwin

package macoslite

// App is unavailable on this platform; New always fails.
type App struct{}

// Canvas is unavailable on this platform.
type Canvas struct{}

// New validates its arguments and returns ErrUnsupported.
func New(o Options, v View) (*App, error) {
	if err := validate(o, v); err != nil {
		return nil, err
	}
	return nil, ErrUnsupported
}

func (*App) Run() error  { return ErrUnsupported }
func (*App) Stop()       {}
func (*App) Invalidate() {}
func (*App) TrimCaches() {}

func (*Canvas) Fill(Rect, Color)                   {}
func (*Canvas) RoundRect(Rect, float64, Color)     {}
func (*Canvas) Text(string, Point, float64, Color) {}
