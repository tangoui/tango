//go:build linux || darwin

package gl

import "fmt"

// loadFuncs loads the function table of h's context, current on the
// calling thread, and checks it provides OpenGL 3.3. Errors wrap
// ErrNoDevice.
func (h *headless) loadFuncs() error {
	f, err := loadCurrentFuncs()
	if err != nil {
		return err
	}
	h.funcs = f
	return nil
}

// loadCurrentFuncs loads the function table of the context current on
// the calling thread and checks it provides OpenGL 3.3. Errors wrap
// ErrNoDevice.
func loadCurrentFuncs() (*glFuncs, error) {
	proc, err := glProc()
	if err != nil {
		return nil, err
	}
	f := new(glFuncs)
	if err := f.load(proc); err != nil {
		return nil, fmt.Errorf("%w (%w)", err, ErrNoDevice)
	}
	major, minor := f.version()
	if major == 0 { // before 3.0 GL_MAJOR_VERSION is unknown
		major, minor, _ = parseVersion(f.GetString(glVERSION))
	}
	if major < 3 || major == 3 && minor < 3 {
		return nil, fmt.Errorf("gl: OpenGL %d.%d (%s), need 3.3: %w", major, minor, f.GetString(glRENDERER), ErrNoDevice)
	}
	return f, nil
}
