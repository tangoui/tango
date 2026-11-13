package uihost

import (
	"errors"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

// FilePicker adapts the native open-file dialog (sys/dialog) to
// ui.UploadW.Picker:
//
//	ui.Upload(c, &files).Picker(uihost.FilePicker(app))
//	ui.ConfigProvider(c).Defaults("Upload", ui.Props{"picker": uihost.FilePicker(app)})
//
// The returned function blocks until the dialog closes; Upload calls it on
// its own goroutine. accept becomes the dialog's file filter (MIME
// wildcards expand to common extensions, see ui.AcceptPatterns); dir picks
// folders. Cancelling returns (nil, nil).
func FilePicker(app *tango.App) func(accept string, multiple, dir bool) ([]string, error) {
	return FilePickerWith(app, dialog.FileOptions{})
}

// FilePickerWith is FilePicker with base dialog options (Parent, Title,
// Directory …); Filters, Multiple and PickFolders are set per call.
func FilePickerWith(app *tango.App, base dialog.FileOptions) func(accept string, multiple, dir bool) ([]string, error) {
	return func(accept string, multiple, dir bool) ([]string, error) {
		o := base
		o.Multiple = multiple
		o.PickFolders = dir
		o.Filters = acceptFilters(accept, base.Filters)
		paths, err := dialog.Open(app, o)
		if errors.Is(err, dialog.ErrCanceled) {
			return nil, nil
		}
		return paths, err
	}
}

// acceptFilters builds the dialog filter of an accept list (none when it
// is empty or only wildcards).
func acceptFilters(accept string, base []dialog.FileFilter) []dialog.FileFilter {
	pats := ui.AcceptPatterns(accept)
	if len(pats) == 0 {
		return base
	}
	return append([]dialog.FileFilter{{Name: accept, Patterns: pats}}, base...)
}

// AcceptDrops hands files dropped on surface s to the Upload dragger (or
// any ui.DropZone) under the drop point of runner r: it calls
// r.DropFiles from s.OnFilesDropped. Create the surface with
// SurfaceOptions.AcceptFiles so the platform reports drops:
//
//	r, s, _ := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{AcceptFiles: true}}, page, ui.RunnerOptions{})
//	uihost.AcceptDrops(r, s)
//
// Platforms that do not report the drop point (0, 0) deliver the files at
// the last pointer position instead.
func AcceptDrops(r *ui.Runner, s *tango.Surface) {
	if r == nil || s == nil {
		return
	}
	s.OnFilesDropped(func(paths []string, x, y int) {
		p := geom.Pt(float32(x), float32(y))
		if x == 0 && y == 0 {
			if fp, ok := r.FileDrag(); ok {
				p = fp
			} else if pp, ok := r.PointerPos(); ok {
				p = pp
			}
		}
		r.DropFiles(p, paths)
	})
}
