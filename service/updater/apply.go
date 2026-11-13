package updater

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"time"
)

// ErrNotWritable is returned by Apply when the folder holding the app
// cannot be written by the user, so the update cannot be installed in
// place (Apply never asks for elevated rights).
var ErrNotWritable = errors.New("updater: the app's folder is not writable")

// Apply installs a Payload from Download once the app has exited, and
// starts the new version. Quit the app promptly after Apply returns nil
// (EventReady): a helper process waits for this process to exit, moves
// the current installation aside, puts the new one in its place and
// starts it. If the new version does not call Confirm within
// ConfirmTimeout (or calls Rollback), the helper restores the previous
// installation and starts it again; Confirm then reports
// StatusRolledBack.
//
// By format (see docs/updates.md):
//
//   - app (macOS): the new bundle is staged next to the running one,
//     cleared of the quarantine attribute and its code signature checked
//     (same Team ID as the running app when that is signed), then
//     swapped in and opened with /usr/bin/open.
//   - appimage, dir (Linux): the new AppImage or install directory
//     replaces $APPIMAGE or the executable's directory.
//   - exe (Windows): the new executable is written next to the running
//     one; the helper renames the old one aside, renames the new one into
//     place and starts it, deleting the old one once confirmed.
//   - nsis, msi (Windows): the helper runs the installer silently
//     (setup.exe /S /D=<dir>, msiexec /i … /qn) and starts the app.
//
// Apply returns ErrUnsupported where the app cannot be updated in place
// (mobile; not running from a bundle, AppImage or matching install), and
// ErrNotWritable when the app's folder is not writable.
func (u *Updater) Apply(p *Payload) error {
	if err := u.apply(p); err != nil {
		return u.fail(err)
	}
	u.emit(Event{Kind: EventReady, Release: p.Release, Patch: p.Patched})
	return nil
}

func (u *Updater) apply(p *Payload) error {
	if p == nil || p.Release == nil {
		return errors.New("updater: nil payload")
	}
	rel := p.Release
	target, err := u.target(rel.Format)
	if err != nil {
		return err
	}
	dir, err := u.stateDir()
	if err != nil {
		return err
	}
	launch, err := launchCommand(rel.Format, target)
	if err != nil {
		return err
	}
	st, err := stage(p, target)
	if err != nil {
		return err
	}
	if rel.Format == FormatDir {
		if err := checkDirLaunch(st.repl, target, launch[0]); err != nil {
			os.RemoveAll(st.work)
			return err
		}
	}
	u.clearState(dir)
	pend := pending{
		From: u.Version, To: rel.Version, Target: target, Work: st.work,
		Time: time.Now().UTC(), Timeout: int(u.confirmTimeout() / time.Second),
	}
	job := helperJob{
		PID: helperPID(), Format: rel.Format, Target: target, Repl: st.repl, Work: st.work,
		State: dir, Timeout: pend.Timeout, InstallerArgs: u.InstallerArgs, Launch: launch,
	}
	if err := writeJSON(filepath.Join(dir, statePending), &pend); err != nil {
		os.RemoveAll(st.work)
		return fmt.Errorf("updater: %w", err)
	}
	if err := startHelper(&job, &pend); err != nil {
		os.RemoveAll(st.work)
		u.clearState(dir)
		return fmt.Errorf("updater: starting the helper: %w", err)
	}
	if pend.HelperDir != "" {
		_ = writeJSON(filepath.Join(dir, statePending), &pend)
	}
	return nil
}

// helperPID is the process the helper waits for (replaced in tests).
var helperPID = os.Getpid

// staged is an update ready for the helper.
type staged struct {
	work string // next to the target; holds repl and, later, the previous version
	repl string // what replaces the target (or the installer to run)
}

// stage prepares p next to target, in a work directory on the same volume
// so the helper's renames are atomic. The work directory is private
// (0700), and what ends up in it is checked against the release again
// (sha256 or tree digest): p.Path may sit in a directory others can
// write to, and must not be swapped between Download's check and here.
func stage(p *Payload, target string) (st staged, err error) {
	rel := p.Release
	if err := checkLocation(target); err != nil {
		return st, err
	}
	if rel.Format == FormatDir {
		home, _ := os.UserHomeDir()
		if err := checkDirTarget(target, home); err != nil {
			return st, err
		}
	}
	parent := filepath.Dir(target)
	st.work, err = os.MkdirTemp(parent, ".tango-update-")
	if err != nil {
		if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EROFS) {
			return st, fmt.Errorf("%w: cannot write to %s (%v). Move %s to a folder you own (such as ~/Applications on macOS) or install the update manually",
				ErrNotWritable, parent, err, filepath.Base(target))
		}
		return st, fmt.Errorf("updater: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(st.work)
		}
	}()
	newDir := filepath.Join(st.work, "new")
	if err := os.Mkdir(newDir, 0o755); err != nil {
		return st, fmt.Errorf("updater: %w", err)
	}
	switch {
	case treeFormat(rel.Format):
		suffix := ""
		if rel.Format == FormatApp {
			suffix = ".app"
		}
		var tree string
		if p.Patched {
			// Copied, not renamed: whoever could write to p.Path could
			// still hold its files open.
			tree = filepath.Join(newDir, filepath.Base(target))
			if err := copyTree(p.Path, tree); err != nil {
				return st, err
			}
			os.RemoveAll(p.Path)
		} else {
			archive := filepath.Join(st.work, filepath.Base(p.Path))
			if err := copyVerified(p.Path, archive, 0o600, rel.Full); err != nil {
				return st, err
			}
			err := extract(archive, newDir)
			os.Remove(archive)
			if err != nil {
				return st, err
			}
			if tree, err = singleTop(newDir, suffix); err != nil {
				return st, err
			}
		}
		digest, err := TreeDigest(tree)
		if err != nil {
			return st, err
		}
		if digest != rel.TreeSHA256 {
			return st, errors.New("updater: unpacked release does not match the manifest's tree_sha256")
		}
		if rel.Format == FormatApp {
			if fi, err := os.Stat(filepath.Join(tree, "Contents", "Info.plist")); err != nil || !fi.Mode().IsRegular() {
				return st, errors.New("updater: app bundle has no Contents/Info.plist")
			}
			if err := prepareBundle(tree, target); err != nil {
				return st, err
			}
		}
		st.repl = tree
	case rel.Format == FormatExe || rel.Format == FormatAppImage:
		st.repl = filepath.Join(newDir, filepath.Base(target))
		if err := copyVerified(p.Path, st.repl, 0o755, rel.Full); err != nil {
			return st, err
		}
	case rel.Format == FormatNSIS || rel.Format == FormatMSI:
		name := filepath.Base(p.Path)
		st.repl = filepath.Join(newDir, name)
		if err := copyVerified(p.Path, st.repl, 0o755, rel.Full); err != nil {
			return st, err
		}
	default:
		return st, fmt.Errorf("updater: unknown format %q", rel.Format)
	}
	return st, nil
}

// checkLocation refuses installations that cannot be replaced in place.
func checkLocation(target string) error {
	if strings.Contains(target, "/AppTranslocation/") {
		return fmt.Errorf("%w: macOS runs %s from a temporary, read-only location (App Translocation); move it to /Applications and start it from there",
			ErrNotWritable, filepath.Base(target))
	}
	if strings.HasPrefix(target, "/Volumes/") && runtime.GOOS == "darwin" {
		if _, err := os.Stat(filepath.Join(filepath.Dir(target), "Applications")); err == nil {
			return fmt.Errorf("%w: %s runs from its disk image; drag it to Applications first", ErrNotWritable, filepath.Base(target))
		}
	}
	return nil
}

// target returns the installation to update for format.
func (u *Updater) target(format string) (string, error) {
	if u.Target != "" {
		return u.Target, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	return targetFor(runtime.GOOS, format, exe, os.Getenv("APPIMAGE"))
}

// targetFor finds the installation of format for executable exe on goos.
func targetFor(goos, format, exe, appImage string) (string, error) {
	switch {
	case goos == "darwin" && format == FormatApp:
		if b, ok := bundleOf(exe); ok {
			return b, nil
		}
	case goos == "linux" && format == FormatAppImage:
		if appImage != "" {
			return appImage, nil
		}
	case goos == "linux" && format == FormatDir:
		if appImage == "" {
			return filepath.Dir(exe), nil
		}
	case goos == "windows" && (format == FormatExe || format == FormatNSIS || format == FormatMSI):
		return exe, nil
	}
	return "", fmt.Errorf("%w (%s format on %s)", ErrUnsupported, format, goos)
}

// launchCommand returns the command that starts the installed app.
func launchCommand(format, target string) ([]string, error) {
	switch format {
	case FormatApp:
		return []string{"/usr/bin/open", target}, nil
	case FormatAppImage, FormatExe, FormatNSIS, FormatMSI:
		return []string{target}, nil
	case FormatDir:
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("updater: %w", err)
		}
		exe, _ = filepath.EvalSymlinks(exe)
		rel, err := filepath.Rel(target, exe)
		if err != nil || !filepath.IsLocal(rel) {
			// Updater.Target names a directory the app does not run from:
			// the new version is started by name from it.
			rel = filepath.Base(exe)
		}
		return []string{filepath.Join(target, rel)}, nil
	}
	return nil, fmt.Errorf("updater: unknown format %q", format)
}

// bundleOf returns the .app bundle that the executable exe runs from
// (Name.app/Contents/MacOS/exe).
func bundleOf(exe string) (string, bool) {
	macos := filepath.Dir(exe)
	contents := filepath.Dir(macos)
	bundle := filepath.Dir(contents)
	if filepath.Base(macos) != "MacOS" || filepath.Base(contents) != "Contents" || !strings.HasSuffix(bundle, ".app") {
		return "", false
	}
	return bundle, true
}

// installerCommand returns the command that runs a Windows installer
// silently into dir (the running app's folder).
func installerCommand(file, dir string, extra []string) (name string, args []string, err error) {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".exe":
		// NSIS: /S silent; /D= must come last and unquoted.
		args = append(append([]string{"/S"}, extra...), "/D="+dir)
		return file, args, nil
	case ".msi":
		return "msiexec", append([]string{"/i", file, "/qn", "/norestart"}, extra...), nil
	}
	return "", nil, fmt.Errorf("updater: %s is not an .exe or .msi installer", filepath.Base(file))
}

// copyVerified copies src to dst and checks the copy (not src, which
// may change meanwhile) against f's size and sha256.
func copyVerified(src, dst string, perm fs.FileMode, f File) error {
	if err := copyFile(src, dst, perm); err != nil {
		return err
	}
	fi, err := os.Stat(dst)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	sum, err := fileDigest(dst)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	if fi.Size() != f.Size || hex.EncodeToString(sum) != strings.ToLower(f.SHA256) {
		os.Remove(dst)
		return fmt.Errorf("updater: %s changed after it was verified", filepath.Base(src))
	}
	return nil
}

// checkDirTarget refuses to treat as an app's install directory (format
// dir, which is replaced as a whole) a directory that plainly is not one:
// the file system root or a top-level directory, a home directory or
// what holds one, or a shared directory such as /usr/bin or ~/.local/bin.
// An app run from there (say, its executable copied into /usr/local/bin)
// would otherwise have the whole directory swapped for the release.
func checkDirTarget(target, home string) error {
	refuse := func(why string) error {
		return fmt.Errorf("%w: %s is %s, not an install directory of its own; install the app in a directory of its own (such as /opt/<app> or ~/.local/opt/<app>) or set Updater.Target",
			ErrUnsupported, target, why)
	}
	if !filepath.IsAbs(target) {
		return refuse("not an absolute path")
	}
	t := filepath.Clean(target)
	rest := filepath.ToSlash(strings.TrimPrefix(t, filepath.VolumeName(t)))
	if strings.Count(strings.Trim(rest, "/"), "/") == 0 {
		return refuse("the root or a top-level directory")
	}
	parent := filepath.ToSlash(filepath.Dir(rest))
	if parent == "/home" || parent == "/Users" {
		return refuse("a home directory")
	}
	if slices.Contains(sharedDirs, rest) {
		return refuse("a shared system directory")
	}
	if home != "" {
		h := filepath.Clean(home)
		if r, err := filepath.Rel(t, h); err == nil && (r == "." || filepath.IsLocal(r)) {
			return refuse("the home directory or holds it")
		}
		for _, d := range homeDirs {
			if t == filepath.Join(h, d) {
				return refuse("a shared directory in the home directory")
			}
		}
	}
	return nil
}

// sharedDirs are system directories that hold many programs' files.
var sharedDirs = []string{
	"/usr/bin", "/usr/sbin", "/usr/lib", "/usr/lib32", "/usr/lib64", "/usr/libexec", "/usr/include",
	"/usr/share", "/usr/src", "/usr/games", "/usr/local", "/usr/local/bin", "/usr/local/sbin",
	"/usr/local/lib", "/usr/local/share", "/usr/local/opt", "/var/lib", "/var/opt", "/var/tmp",
	"/snap/bin", "/etc/opt", "/opt/bin",
}

// homeDirs are directories in a home directory that hold more than one
// app or the user's own files.
var homeDirs = []string{
	"bin", ".local", ".local/bin", ".local/lib", ".local/share", ".local/opt", ".config", "opt",
	"apps", "Apps", "Applications", "Desktop", "Downloads", "Documents",
}

// checkDirLaunch checks that the new install directory tree holds the
// executable the helper will start (exe, under target), when the app
// runs from target: a release missing it would not start.
func checkDirLaunch(tree, target, exe string) error {
	rel, err := filepath.Rel(target, exe)
	if err != nil || !filepath.IsLocal(rel) {
		return nil // Updater.Target names a directory the app does not run from
	}
	if real, err := filepath.EvalSymlinks(target); err == nil && real != target {
		return nil
	}
	fi, err := os.Lstat(filepath.Join(tree, rel))
	// Windows file modes carry no execute bit (FormatDir is a Linux format;
	// this only matters for tests run there).
	if err != nil || !fi.Mode().IsRegular() || runtime.GOOS != "windows" && fi.Mode().Perm()&0o100 == 0 {
		return fmt.Errorf("updater: the release has no executable %s, which the app runs as", filepath.ToSlash(rel))
	}
	return nil
}

// copyTree copies the directory src to dst (symlinks as symlinks).
func copyTree(src, dst string) error {
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		switch t := d.Type(); {
		case t.IsDir():
			return os.MkdirAll(out, 0o755)
		case t&fs.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(l, out)
		case t.IsRegular():
			fi, err := d.Info()
			if err != nil {
				return err
			}
			perm := fs.FileMode(0o644)
			if fi.Mode().Perm()&0o100 != 0 {
				perm = 0o755
			}
			return copyFile(p, out, perm)
		default:
			return fmt.Errorf("%s is not a file, directory or symlink", p)
		}
	})
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	return nil
}

func copyFile(src, dst string, perm fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	_, err = io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(dst, perm)
	}
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	return nil
}
