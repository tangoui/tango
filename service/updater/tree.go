package updater

import (
	"archive/tar"
	"cmp"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Directory installs (a macOS .app bundle, a Linux install directory) are
// described by a tree digest and patched with a tree patch.

// TreeDigest returns the hex SHA-256 digest of the directory tree at dir:
// of a listing, in path order, of every directory, symlink (with its
// target) and regular file (with its executable bit and content digest)
// below dir. Ownership, times and other permission bits do not count, so
// a tree unpacked from an archive and the tree it was packed from have
// the same digest. Other file types are errors.
func TreeDigest(dir string) (string, error) {
	entries, err := scanTree(dir)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, e := range entries {
		switch e.Type {
		case "dir":
			fmt.Fprintf(h, "d %s\n", e.Path)
		case "symlink":
			fmt.Fprintf(h, "l %s %s\n", e.Path, e.Link)
		default:
			x := "-"
			if e.Exec {
				x = "x"
			}
			fmt.Fprintf(h, "f %s %s %s\n", e.Path, x, e.SHA256)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// treeEntry is one entry of a tree, and of a tree patch's index.
type treeEntry struct {
	Path   string `json:"path"` // slash-separated, relative to the root
	Type   string `json:"type"` // dir | file | symlink
	Link   string `json:"link,omitempty"`
	Exec   bool   `json:"exec,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`

	// In a tree patch, how a file is made: "copy" the old file From,
	// "patch" the old file From with Blob, or take the "data" in Blob.
	Op   string `json:"op,omitempty"`
	From string `json:"from,omitempty"`
	Blob string `json:"blob,omitempty"`
}

func scanTree(dir string) ([]treeEntry, error) {
	var entries []treeEntry
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			if !d.IsDir() {
				return fmt.Errorf("%s is not a directory", dir)
			}
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		e := treeEntry{Path: filepath.ToSlash(rel)}
		switch t := d.Type(); {
		case t.IsDir():
			e.Type = "dir"
		case t&fs.ModeSymlink != 0:
			e.Type = "symlink"
			if e.Link, err = os.Readlink(p); err != nil {
				return err
			}
			e.Link = filepath.ToSlash(e.Link)
		case t.IsRegular():
			fi, err := d.Info()
			if err != nil {
				return err
			}
			sum, err := fileDigest(p)
			if err != nil {
				return err
			}
			e.Type, e.Exec, e.Size, e.SHA256 = "file", fi.Mode().Perm()&0o100 != 0, fi.Size(), hex.EncodeToString(sum)
		default:
			return fmt.Errorf("%s is not a file, directory or symlink", p)
		}
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("updater: %w", err)
	}
	// WalkDir visits in lexical order of each directory's names; sort by
	// the full path so the order does not depend on the walk.
	slices.SortFunc(entries, func(a, b treeEntry) int { return strings.Compare(a.Path, b.Path) })
	return entries, nil
}

// treePatchIndex is the first entry, "index.json", of a tree patch.
type treePatchIndex struct {
	Format  string      `json:"format"` // treePatchFormat
	OldTree string      `json:"old_tree"`
	NewTree string      `json:"new_tree"`
	Entries []treeEntry `json:"entries"`
}

const treePatchFormat = "tango-tree-patch-1"

// smallFile is the size below which tree patches store files whole.
const smallFile = 1024

// DiffTree writes to w a tree patch that turns the tree at oldDir into the
// tree at newDir. A tree patch is a gzip-compressed tar archive: an index
// (index.json) listing the new tree, where each file is copied unchanged
// from an old file with the same content, patched (Diff) from the old file
// at the same path, or stored whole; then the patches and stored files.
func DiffTree(oldDir, newDir string, w io.Writer) error {
	oldEntries, err := scanTree(oldDir)
	if err != nil {
		return err
	}
	newEntries, err := scanTree(newDir)
	if err != nil {
		return err
	}
	oldDigest, err := TreeDigest(oldDir)
	if err != nil {
		return err
	}
	newDigest, err := TreeDigest(newDir)
	if err != nil {
		return err
	}
	byPath := map[string]treeEntry{}
	bySum := map[string]string{}
	for _, e := range oldEntries {
		byPath[e.Path] = e
		if e.Type == "file" {
			if _, ok := bySum[e.SHA256]; !ok {
				bySum[e.SHA256] = e.Path
			}
		}
	}
	type blob struct {
		name string
		data []byte
	}
	var blobs []blob
	for i := range newEntries {
		e := &newEntries[i]
		if e.Type != "file" {
			continue
		}
		if from, ok := bySum[e.SHA256]; ok {
			e.Op, e.From = "copy", from
			continue
		}
		data, err := os.ReadFile(filepath.Join(newDir, filepath.FromSlash(e.Path)))
		if err != nil {
			return fmt.Errorf("updater: %w", err)
		}
		e.Op, e.Blob = "data", fmt.Sprintf("blob/%d", len(blobs))
		if o, ok := byPath[e.Path]; ok && o.Type == "file" && e.Size >= smallFile {
			old, err := os.ReadFile(filepath.Join(oldDir, filepath.FromSlash(o.Path)))
			if err != nil {
				return fmt.Errorf("updater: %w", err)
			}
			p, err := Diff(old, data)
			if err != nil {
				return err
			}
			// Keep the patch only when it beats storing the file (which
			// the outer gzip also compresses) by a margin.
			if len(p) < len(data)*3/4 {
				e.Op, e.From, data = "patch", o.Path, p
			}
		}
		blobs = append(blobs, blob{e.Blob, data})
	}
	index, err := json.Marshal(treePatchIndex{treePatchFormat, oldDigest, newDigest, newEntries})
	if err != nil {
		return err
	}
	gz, _ := gzip.NewWriterLevel(w, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	put := func(name string, b []byte) error {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		_, err := tw.Write(b)
		return err
	}
	if err := put("index.json", index); err != nil {
		return err
	}
	for _, b := range blobs {
		if err := put(b.name, b.data); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// maxTreePatch bounds what PatchTree reads from a tree patch.
const maxTreePatch = 4 << 30

// PatchTree builds in outDir, which must not exist, the tree that the tree
// patch r makes from the tree at oldDir, and returns the new tree's
// digest after checking it against the one the patch records. Old files
// are read and new ones written through os.Root, so neither the patch nor
// symlinks in either tree reach outside them. outDir is removed on error.
func PatchTree(oldDir string, r io.Reader, outDir string) (digest string, err error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return "", fmt.Errorf("updater: tree patch: %w", err)
	}
	tr := tar.NewReader(io.LimitReader(gz, maxTreePatch))
	h, err := tr.Next()
	if err != nil || h.Name != "index.json" {
		return "", errors.New("updater: tree patch has no index")
	}
	var idx treePatchIndex
	if err := json.NewDecoder(io.LimitReader(tr, 64<<20)).Decode(&idx); err != nil || idx.Format != treePatchFormat {
		return "", errors.New("updater: tree patch has a bad index")
	}
	blobs := map[string][]byte{}
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("updater: tree patch: %w", err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return "", fmt.Errorf("updater: tree patch: %w", err)
		}
		blobs[h.Name] = b
	}

	old, err := os.OpenRoot(oldDir)
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	defer old.Close()
	if err := os.Mkdir(outDir, 0o755); err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(outDir)
		}
	}()
	root, err := os.OpenRoot(outDir)
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	defer root.Close()

	for _, e := range idx.Entries {
		if err := patchEntry(old, root, e, blobs); err != nil {
			return "", err
		}
	}
	if err := checkSymlinks(outDir, root); err != nil {
		return "", err
	}
	digest, err = TreeDigest(outDir)
	if err != nil {
		return "", err
	}
	if digest != idx.NewTree {
		return "", errors.New("updater: patched tree has the wrong digest")
	}
	return digest, nil
}

func patchEntry(old, root *os.Root, e treeEntry, blobs map[string][]byte) error {
	if !fs.ValidPath(e.Path) || e.Path == "." || strings.Contains(e.Path, `\`) {
		return fmt.Errorf("updater: tree patch entry %q escapes the tree", e.Path)
	}
	switch e.Type {
	case "dir":
		return wrap(root.MkdirAll(e.Path, 0o755))
	case "symlink":
		if !symlinkInside(root, e.Path, e.Link) {
			return fmt.Errorf("updater: tree patch symlink %q → %q points outside the tree", e.Path, e.Link)
		}
		if err := mkParent(root, e.Path); err != nil {
			return err
		}
		return wrap(root.Symlink(e.Link, e.Path))
	case "file":
	default:
		return fmt.Errorf("updater: tree patch entry %q has type %q", e.Path, e.Type)
	}
	var data []byte
	readOld := func() ([]byte, error) {
		if !fs.ValidPath(e.From) {
			return nil, fmt.Errorf("updater: tree patch source %q escapes the tree", e.From)
		}
		f, err := old.Open(e.From)
		if err != nil {
			return nil, ErrPatchBase
		}
		defer f.Close()
		return io.ReadAll(f)
	}
	switch e.Op {
	case "copy":
		b, err := readOld()
		if err != nil {
			return err
		}
		if sum := sha256.Sum256(b); hex.EncodeToString(sum[:]) != e.SHA256 {
			return ErrPatchBase
		}
		data = b
	case "patch":
		b, err := readOld()
		if err != nil {
			return err
		}
		p, ok := blobs[e.Blob]
		if !ok {
			return fmt.Errorf("updater: tree patch lacks %s", e.Blob)
		}
		if data, err = PatchMax(b, p, e.Size); err != nil {
			return err
		}
	case "data":
		b, ok := blobs[e.Blob]
		if !ok {
			return fmt.Errorf("updater: tree patch lacks %s", e.Blob)
		}
		data = b
	default:
		return fmt.Errorf("updater: tree patch entry %q has op %q", e.Path, e.Op)
	}
	if err := mkParent(root, e.Path); err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if e.Exec {
		mode = 0o755
	}
	f, err := root.OpenFile(e.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return wrap(err)
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = root.Chmod(e.Path, mode)
	}
	return wrap(err)
}

// PackTree writes the directory dir as a gzip-compressed tar archive to
// out, with dir's base name as the top directory (Notes.app/...), the
// layout Apply expects for macOS bundles and Linux install directories.
// Entries are in path order with fixed times and owners, so packing the
// same tree twice gives the same archive.
func PackTree(dir, out string) (err error) {
	entries, err := scanTree(dir)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}()
	gz, _ := gzip.NewWriterLevel(f, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	top := filepath.Base(dir)
	epoch := time.Unix(0, 0)
	if err := tw.WriteHeader(&tar.Header{Name: top + "/", Typeflag: tar.TypeDir, Mode: 0o755, ModTime: epoch}); err != nil {
		return err
	}
	for _, e := range entries {
		h := &tar.Header{Name: path.Join(top, e.Path), ModTime: epoch, Format: tar.FormatPAX}
		switch e.Type {
		case "dir":
			h.Name += "/"
			h.Typeflag, h.Mode = tar.TypeDir, 0o755
		case "symlink":
			h.Typeflag, h.Linkname, h.Mode = tar.TypeSymlink, e.Link, 0o777
		default:
			h.Typeflag, h.Size, h.Mode = tar.TypeReg, e.Size, 0o644
			if e.Exec {
				h.Mode = 0o755
			}
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if e.Type == "file" {
			src, err := os.Open(filepath.Join(dir, filepath.FromSlash(e.Path)))
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, src)
			src.Close()
			if err != nil {
				return err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// Unpack extracts archive (.tar.gz, .tgz or .zip holding one top-level
// directory, as PackTree writes) into the existing directory dst and
// returns the path of that top directory. Entries may not escape dst.
func Unpack(archive, dst string) (string, error) {
	if err := extract(archive, dst); err != nil {
		return "", err
	}
	return singleTop(dst, "")
}

// singleTop returns the one directory in dir (ignoring __MACOSX and dot
// files), which must end in suffix.
func singleTop(dir, suffix string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("updater: %w", err)
	}
	var tops []string
	for _, e := range entries {
		if e.Name() == "__MACOSX" || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !e.IsDir() || !strings.HasSuffix(e.Name(), suffix) {
			return "", fmt.Errorf("updater: archive holds %q besides the %s directory", e.Name(), cmp.Or(suffix, "top"))
		}
		tops = append(tops, e.Name())
	}
	if len(tops) != 1 {
		return "", fmt.Errorf("updater: archive holds %d top-level directories, want 1", len(tops))
	}
	return filepath.Join(dir, tops[0]), nil
}

// readAllFile reads a file, for patching (whole files fit in memory).
func readAllFile(p string) ([]byte, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("updater: %w", err)
	}
	return b, nil
}
