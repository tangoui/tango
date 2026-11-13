package windows

import (
	"strings"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/rolenames"
)

// MessageBoxW styles and results (winuser.h).
const (
	mbOK              = 0x00000000
	mbOKCancel        = 0x00000001
	mbYesNoCancel     = 0x00000003
	mbYesNo           = 0x00000004
	mbIconError       = 0x00000010
	mbIconQuestion    = 0x00000020
	mbIconWarning     = 0x00000030
	mbIconInformation = 0x00000040
	mbTaskModal       = 0x00002000

	idOK     = 1
	idCancel = 2
	idYes    = 6
	idNo     = 7
)

// messageButtons returns opts.Buttons, or a single localized "OK".
func messageButtons(b []string) []string {
	if len(b) == 0 {
		return []string{rolenames.Text("win.ok")}
	}
	return b
}

// messageBoxStyle is the MessageBoxW fallback used when task dialogs are
// unavailable: it cannot label buttons, so it picks the standard set with
// the same count (more than three buttons are truncated to three).
func messageBoxStyle(kind platform.MessageKind, n int) uint32 {
	var s uint32
	switch {
	case n <= 1:
		s = mbOK
	case n == 2:
		if kind == platform.MessageQuestion {
			s = mbYesNo
		} else {
			s = mbOKCancel
		}
	default:
		s = mbYesNoCancel
	}
	switch kind {
	case platform.MessageWarning:
		s |= mbIconWarning
	case platform.MessageError:
		s |= mbIconError
	case platform.MessageQuestion:
		s |= mbIconQuestion
	default:
		s |= mbIconInformation
	}
	return s
}

// messageBoxIndex maps a MessageBoxW result to a button index.
func messageBoxIndex(ret uintptr, n int) (int, error) {
	switch int(ret) {
	case idOK, idYes:
		return 0, nil
	case idNo:
		return 1, nil
	case idCancel:
		// The fallback box shows the system's OK/Cancel labels, not the
		// caller's, and Esc / the close box also answer IDCANCEL: both are
		// a dismissal. The OK box's close button means OK.
		if n <= 1 {
			return 0, nil
		}
		return -1, platform.ErrCanceled
	}
	return -1, platform.ErrCanceled
}

// Task dialog buttons get IDs from firstButtonID up.
const firstButtonID = 100

// taskDialogIndex maps a TaskDialogIndirect pressed ID to a button index.
// Closing the dialog (Esc / close box) with several buttons is a cancel;
// with a single button it is that button.
func taskDialogIndex(pressed int32, n int) (int, error) {
	if pressed >= firstButtonID && int(pressed-firstButtonID) < n {
		return int(pressed - firstButtonID), nil
	}
	if n <= 1 {
		return 0, nil
	}
	return -1, platform.ErrCanceled
}

// Task dialog main icons (commctrl.h: MAKEINTRESOURCEW(-1), (-2), (-3)).
const (
	tdWarningIcon     = 0xFFFF
	tdErrorIcon       = 0xFFFE
	tdInformationIcon = 0xFFFD
)

func taskDialogIcon(kind platform.MessageKind) uintptr {
	switch kind {
	case platform.MessageWarning:
		return tdWarningIcon
	case platform.MessageError:
		return tdErrorIcon
	}
	return tdInformationIcon
}

// filterSpec is one COMDLG_FILTERSPEC entry, as strings.
type filterSpec struct{ Name, Spec string }

// normalizePattern turns "png", ".png" and "*.png" into "*.png" and "*"
// into "*.*".
func normalizePattern(p string) string {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return ""
	case p == "*" || p == "*.*":
		return "*.*"
	case strings.HasPrefix(p, "*"):
		return p
	case strings.HasPrefix(p, "."):
		return "*" + p
	}
	return "*." + p
}

// specPatterns returns f's patterns in normalized form, blanks dropped.
func specPatterns(f platform.FileFilter) []string {
	var pats []string
	for _, p := range f.Patterns {
		if n := normalizePattern(p); n != "" {
			pats = append(pats, n)
		}
	}
	return pats
}

// filterSpecs builds the COMDLG_FILTERSPEC list for IFileDialog::SetFileTypes:
// one entry per filter that has a pattern left after normalization, its
// patterns joined with ";" and its name defaulting to that pattern list.
func filterSpecs(filters []platform.FileFilter) []filterSpec {
	var specs []filterSpec
	for i := range filters {
		pats := specPatterns(filters[i])
		if len(pats) == 0 {
			continue
		}
		spec := filterSpec{Name: filters[i].Name, Spec: strings.Join(pats, ";")}
		if spec.Name == "" {
			spec.Name = spec.Spec
		}
		specs = append(specs, spec)
	}
	return specs
}

// defaultExtension is what a save dialog appends when the user types no
// extension (IFileDialog::SetDefaultExtension, no dot). It comes from the
// first pattern of the first filter, and only if that pattern names one
// concrete extension; otherwise there is none.
func defaultExtension(filters []platform.FileFilter) string {
	if len(filters) == 0 {
		return ""
	}
	pats := specPatterns(filters[0])
	if len(pats) == 0 {
		return ""
	}
	ext, ok := strings.CutPrefix(pats[0], "*.")
	if !ok || strings.ContainsAny(ext, "*?") {
		return ""
	}
	return ext
}

// FILEOPENDIALOGOPTIONS bits used here (shobjidl_core.h).
const (
	fosOverwritePrompt  = 0x00000002 // FOS_OVERWRITEPROMPT
	fosNoChangeDir      = 0x00000008 // FOS_NOCHANGEDIR
	fosPickFolders      = 0x00000020 // FOS_PICKFOLDERS
	fosForceFileSystem  = 0x00000040 // FOS_FORCEFILESYSTEM
	fosAllowMultiselect = 0x00000200 // FOS_ALLOWMULTISELECT
	fosPathMustExist    = 0x00000800 // FOS_PATHMUSTEXIST
	fosFileMustExist    = 0x00001000 // FOS_FILEMUSTEXIST
	fosForceShowHidden  = 0x10000000 // FOS_FORCESHOWHIDDEN
)

// Both dialogs return file system paths only, require existing folders
// and leave the process's current directory alone. "New folder" is always
// offered by the shell, so FileOptions.CanCreateDirs maps to nothing.
const fosCommon = fosForceFileSystem | fosPathMustExist | fosNoChangeDir

// optionBits ors together the flags whose condition holds.
func optionBits(base uint32, conds ...optionIf) uint32 {
	for _, c := range conds {
		if c.on {
			base |= c.flag
		}
	}
	return base
}

// optionIf is one conditional flag for optionBits.
type optionIf struct {
	on   bool
	flag uint32
}

func openDialogOptions(o platform.FileOptions) uint32 {
	return optionBits(fosCommon,
		optionIf{o.PickFolders, fosPickFolders},
		optionIf{!o.PickFolders, fosFileMustExist}, // files must exist; folders are picked as is
		optionIf{o.Multiple, fosAllowMultiselect},
		optionIf{o.ShowHidden, fosForceShowHidden})
}

func saveDialogOptions(o platform.FileOptions) uint32 {
	return optionBits(fosCommon|fosOverwritePrompt,
		optionIf{o.ShowHidden, fosForceShowHidden})
}
