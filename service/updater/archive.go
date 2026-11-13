package updater

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// extract unpacks the .zip, .tar.gz or .tgz archive at file into the
// existing directory dst. Entries must stay inside dst: names that are
// absolute or contain "..", symlinks that point outside the archive's
// tree, and other special files are errors. Writes go through an os.Root,
// so even a symlink created earlier in the archive cannot redirect them.
// Regular files keep their permission bits (executables stay executable).
func extract(file, dst string) error {
	root, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer root.Close()
	lower := strings.ToLower(file)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		err = extractZip(file, root)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		err = extractTarGz(file, root)
	default:
		return fmt.Errorf("updater: %s is not a .zip or .tar.gz archive", path.Base(file))
	}
	if err != nil {
		return err
	}
	return checkSymlinks(dst, root)
}

func extractZip(file string, root *os.Root) error {
	zr, err := zip.OpenReader(file)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if err := extractEntry(root, f.Name, f.Mode(), func() (io.ReadCloser, error) { return f.Open() }); err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(file string, root *os.Root) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("updater: %w", err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("updater: %w", err)
		}
		var mode fs.FileMode
		switch h.Typeflag {
		case tar.TypeDir:
			mode = fs.ModeDir | fs.FileMode(h.Mode).Perm()
		case tar.TypeReg:
			mode = fs.FileMode(h.Mode).Perm()
		case tar.TypeSymlink:
			mode = fs.ModeSymlink
		case tar.TypeXGlobalHeader:
			continue
		default:
			return fmt.Errorf("updater: archive entry %q has unsupported type %q", h.Name, h.Typeflag)
		}
		linkname := h.Linkname
		open := func() (io.ReadCloser, error) {
			if mode&fs.ModeSymlink != 0 {
				return io.NopCloser(strings.NewReader(linkname)), nil
			}
			return io.NopCloser(tr), nil
		}
		if err := extractEntry(root, h.Name, mode, open); err != nil {
			return err
		}
	}
}

// extractEntry writes one archive entry. For symlinks, open yields the
// link target (as zip stores it).
func extractEntry(root *os.Root, name string, mode fs.FileMode, open func() (io.ReadCloser, error)) error {
	clean := strings.TrimSuffix(name, "/")
	if !fs.ValidPath(clean) || clean == "." || strings.Contains(clean, `\`) {
		return fmt.Errorf("updater: archive entry %q escapes the archive", name)
	}
	switch {
	case mode.IsDir():
		return wrap(root.MkdirAll(clean, dirPerm(mode)))
	case mode&fs.ModeSymlink != 0:
		rc, err := open()
		if err != nil {
			return wrap(err)
		}
		target, err := io.ReadAll(io.LimitReader(rc, 4096))
		rc.Close()
		if err != nil {
			return wrap(err)
		}
		if !symlinkInside(root, clean, string(target)) {
			return fmt.Errorf("updater: archive symlink %q → %q points outside the archive", name, target)
		}
		if err := mkParent(root, clean); err != nil {
			return err
		}
		return wrap(root.Symlink(string(target), clean))
	case mode.IsRegular():
		if err := mkParent(root, clean); err != nil {
			return err
		}
		rc, err := open()
		if err != nil {
			return wrap(err)
		}
		defer rc.Close()
		out, err := root.OpenFile(clean, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode(mode))
		if err != nil {
			return wrap(err)
		}
		_, err = io.Copy(out, rc)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			// OpenFile's permissions are masked by the umask.
			err = root.Chmod(clean, fileMode(mode))
		}
		return wrap(err)
	}
	return fmt.Errorf("updater: archive entry %q is not a file, directory or symlink", name)
}

func mkParent(root *os.Root, name string) error {
	if dir := path.Dir(name); dir != "." {
		return wrap(root.MkdirAll(dir, 0o755))
	}
	return nil
}

// maxLinkHops bounds how many symlinks resolving one path may follow.
const maxLinkHops = 40

// symlinkInside reports whether a symlink at name (slash-separated,
// relative to root) pointing at target resolves inside root. It resolves
// the way the system will: the link's directory is located through the
// links already in root (so "a" → "." followed by "a/b" → ".." puts b at
// the top, where ".." leaves root), and links met in target are followed.
// Components that do not exist yet count as plain directories;
// checkSymlinks repeats the check once the whole tree is written.
func symlinkInside(root *os.Root, name, target string) bool {
	if target == "" || path.IsAbs(target) || strings.Contains(target, `\`) {
		return false
	}
	hops := 0
	dir, ok := resolveIn(root, nil, path.Dir(name), &hops)
	if !ok {
		return false
	}
	_, ok = resolveIn(root, dir, target, &hops)
	return ok
}

// resolveIn resolves the slash-separated relative path p from cur (the
// components of a directory below root: existing non-links, or not yet
// existing) and reports whether it stays inside root.
func resolveIn(root *os.Root, cur []string, p string, hops *int) ([]string, bool) {
	if path.IsAbs(p) || strings.Contains(p, `\`) {
		return nil, false
	}
	cur = slices.Clip(cur)
	for _, c := range strings.Split(p, "/") {
		switch c {
		case "", ".":
			continue
		case "..":
			if len(cur) == 0 {
				return nil, false
			}
			cur = slices.Clip(cur[:len(cur)-1])
			continue
		}
		next := append(cur, c)
		rel := strings.Join(next, "/")
		fi, err := root.Lstat(rel)
		if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
			cur = next // a directory or file, or not there (yet)
			continue
		}
		if *hops++; *hops > maxLinkHops {
			return nil, false
		}
		link, err := root.Readlink(rel)
		if err != nil {
			return nil, false
		}
		var ok bool
		if cur, ok = resolveIn(root, cur, filepath.ToSlash(link), hops); !ok {
			return nil, false
		}
	}
	return cur, true
}

// checkSymlinks verifies, once the tree at dir (opened as root) is
// complete, that every symlink in it resolves inside it: a link checked
// when it was written can be redirected by links written after it.
func checkSymlinks(dir string, root *os.Root) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return wrap(err)
		}
		if d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return wrap(err)
		}
		rel = filepath.ToSlash(rel)
		link, err := root.Readlink(rel)
		if err != nil {
			return wrap(err)
		}
		if !symlinkInside(root, rel, filepath.ToSlash(link)) {
			return fmt.Errorf("updater: symlink %q → %q points outside the tree", rel, link)
		}
		return nil
	})
}

func dirPerm(m fs.FileMode) fs.FileMode {
	if p := m.Perm(); p != 0 {
		return p | 0o700
	}
	return 0o755
}

func fileMode(m fs.FileMode) fs.FileMode {
	if p := m.Perm(); p != 0 {
		return p | 0o600
	}
	return 0o644
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("updater: extracting: %w", err)
}
