// Package kv is a small thread-safe key-value store for app settings and
// state: kept in memory and, optionally, persisted to a JSON file.
//
// Values are anything encoding/json can encode. Once persisted and loaded
// again they come back as JSON decodes them into an any (numbers as
// float64, objects as map[string]any), so use the store for simple values.
//
// A *Store can be bound to the page with bind.Register or service.Bind; its
// exported methods are Get, Has, Set, Delete, Keys and Clear.
package kv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"
)

// Store is a key-value store. Its methods are safe for concurrent use.
type Store struct {
	path string // "" for memory only

	mu sync.RWMutex
	m  map[string]any
}

// New returns a store persisted to the JSON file at path, loading it if it
// exists. Every change rewrites the file atomically (a temporary file in the
// same directory renamed over it), so a crash leaves either the old or the
// new contents. An empty path returns a memory-only store.
func New(path string) (*Store, error) {
	s := &Store{path: path, m: map[string]any{}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("kv: %w", err)
	}
	if len(b) > 0 {
		if err := json.Unmarshal(b, &s.m); err != nil {
			return nil, fmt.Errorf("kv: %s: %w", path, err)
		}
		if s.m == nil { // file held "null"
			s.m = map[string]any{}
		}
	}
	return s, nil
}

// Get returns the value stored under key, or nil if there is none.
func (s *Store) Get(key string) any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[key]
}

// Has reports whether a value is stored under key.
func (s *Store) Has(key string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.m[key]
	return ok
}

// Set stores value under key. If the store is persisted and the file
// cannot be written, the store is left unchanged.
func (s *Store) Set(key string, value any) error {
	if _, err := json.Marshal(value); err != nil {
		return fmt.Errorf("kv: %q: %w", key, err)
	}
	return s.update(func(m map[string]any) { m[key] = value })
}

// Delete removes key. Deleting a missing key is not an error.
func (s *Store) Delete(key string) error {
	s.mu.RLock()
	_, ok := s.m[key]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	return s.update(func(m map[string]any) { delete(m, key) })
}

// Keys returns the stored keys in sorted order.
func (s *Store) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Sorted(maps.Keys(s.m))
}

// Clear removes every key.
func (s *Store) Clear() error {
	return s.update(func(m map[string]any) { clear(m) })
}

// update applies fn to a copy of the map, persists the copy and only then
// makes it current, so a failed write changes nothing.
func (s *Store) update(fn func(map[string]any)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := maps.Clone(s.m)
	fn(next)
	if s.path != "" {
		if err := writeJSON(s.path, next); err != nil {
			return fmt.Errorf("kv: %w", err)
		}
	}
	s.m = next
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(b, '\n'), 0o600)
}

// WriteFileAtomic writes data to path through a temporary file in the same
// directory that is synced and then renamed over path. Missing parent
// directories are created.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if err = os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
