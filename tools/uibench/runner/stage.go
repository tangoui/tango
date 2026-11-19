package main

// Private copies of the verified binaries. The manifest is verified once
// at startup, but a run lasts tens of minutes on a shared machine: if
// anyone ran build.sh meanwhile, later rounds would launch new binaries
// under the old manifest. So the runner copies the four binaries into a
// directory of its own (0700), hashing while copying, and launches only
// those copies; each launch hashes its copy again and records that sha.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// stagedDir owns the private directory. cleanup may run from the signal
// handler while main is staging: the mutex orders the two, and a
// directory created after cleanup is removed at once instead of kept.
type stagedDir struct {
	mu     sync.Mutex
	dir    string
	closed bool
}

// staged is the runner's private directory, removed on every exit path
// (normal end, fatal, SIGINT/SIGTERM after the scene programs are stopped).
var staged stagedDir

func (s *stagedDir) set(dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		os.RemoveAll(dir)
		return errors.New("runner stopping: binaries not staged")
	}
	s.dir = dir
	return nil
}

// cleanup removes the directory; later sets are refused.
func (s *stagedDir) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.dir != "" {
		os.RemoveAll(s.dir)
		s.dir = ""
	}
}

// drop removes a directory whose staging failed.
func (s *stagedDir) drop(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.RemoveAll(dir)
	if s.dir == dir {
		s.dir = ""
	}
}

// stageBinaries copies the manifest's binaries from bin into a new private
// directory registered with reg and returns it. A binary whose bytes are
// not the manifest's (changed since verification) fails the staging.
func stageBinaries(bin string, m Manifest, reg *stagedDir) (string, error) {
	dir, err := os.MkdirTemp("", "uibench-bin-")
	if err != nil {
		return "", err
	}
	if err := reg.set(dir); err != nil {
		return "", err
	}
	fail := func(err error) (string, error) { reg.drop(dir); return "", err }
	if err := os.Chmod(dir, 0o700); err != nil {
		return fail(err)
	}
	for _, name := range append(append([]string{}, scenePrograms...), "runner") {
		e := m.entry(name)
		if e == nil {
			return fail(fmt.Errorf("manifest has no %s", name))
		}
		// Same file names as in bin/: MyGo takes its app name and resource
		// directory from its executable's name and directory.
		sum, err := copyHashed(filepath.Join(bin, e.File), filepath.Join(dir, e.File))
		if err != nil {
			return fail(fmt.Errorf("%s: %v", name, err))
		}
		if sum != e.SHA256 {
			return fail(fmt.Errorf("%s: SHA-256 %s changed since verification (manifest %s): did build.sh run? Start again", name, short(sum), short(e.SHA256)))
		}
	}
	return dir, nil
}

// copyHashed copies src to a new file dst (0700) and returns the SHA-256
// of the bytes copied.
func copyHashed(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(out, io.TeeReader(in, h)); err != nil {
		out.Close()
		return "", err
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// launchBuild hashes the copy fw is about to be launched from and returns
// the round's build line with that sha; a copy that is not the manifest's
// fails the round.
func launchBuild(cfg config, fw string) (string, error) {
	e := cfg.manifest.entry(fw)
	if e == nil {
		return "", fmt.Errorf("manifest has no %s", fw)
	}
	sum, err := fileSHA256(filepath.Join(cfg.bin, fw))
	if err != nil {
		return "", err
	}
	line := cfg.manifest.buildLine(fw, sum)
	if sum != e.SHA256 {
		return line, fmt.Errorf("%s: launch copy SHA-256 %s is not the manifest's %s", fw, short(sum), short(e.SHA256))
	}
	return line, nil
}

// annotateWhy marks a verdict's reason with what limits its adoption: a
// runner that is not the manifest's, or a dirty build.
func annotateWhy(why string, m Manifest, unverifiedRunner bool) string {
	if m.dirty() {
		why += " (dirty build)"
	}
	if unverifiedRunner {
		why = "UNVERIFIED RUNNER (not adoptable): " + why
	}
	return why
}
