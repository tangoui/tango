package devhost

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"sync"
)

// Entry is one Persist value in a snapshot.
type Entry struct {
	Key  string // Type + "#" + its index among values of that type
	Type string // Go type of the pointed-to value, e.g. "main.formState"
	Data []byte // gob encoding
}

// Registry records the values passed to uihost.Persist.
type Registry struct {
	mu     sync.Mutex
	items  []regItem
	counts map[string]int
}

type regItem struct {
	key, typ string
	ptr      reflect.Value
}

// Add registers ptr (a non-nil pointer) and returns its key and type name.
func (r *Registry) Add(ptr any) (key, typ string, err error) {
	v := reflect.ValueOf(ptr)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return "", "", fmt.Errorf("devhost: Persist needs a non-nil pointer, got %T", ptr)
	}
	typ = v.Type().Elem().String()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counts == nil {
		r.counts = map[string]int{}
	}
	key = typ + "#" + strconv.Itoa(r.counts[typ])
	r.counts[typ]++
	r.items = append(r.items, regItem{key, typ, v})
	return key, typ, nil
}

// Len returns the number of registered values.
func (r *Registry) Len() int { r.mu.Lock(); defer r.mu.Unlock(); return len(r.items) }

// Encode gob-encodes every registered value. Values that fail to encode
// are skipped and reported in the error.
func (r *Registry) Encode() ([]Entry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Entry
	var errs []error
	for _, it := range r.items {
		var b bytes.Buffer
		if err := gob.NewEncoder(&b).EncodeValue(it.ptr); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", it.typ, err))
			continue
		}
		out = append(out, Entry{Key: it.key, Type: it.typ, Data: b.Bytes()})
	}
	return out, errors.Join(errs...)
}

// Decode fills ptr from the entry with key in entries. It reports whether
// there was one. On a decode error (the type changed incompatibly) ptr is
// left untouched.
func Decode(entries []Entry, key string, ptr any) (bool, error) {
	for _, e := range entries {
		if e.Key != key {
			continue
		}
		v := reflect.ValueOf(ptr)
		if v.Kind() != reflect.Pointer || v.IsNil() {
			return true, fmt.Errorf("devhost: Decode needs a non-nil pointer, got %T", ptr)
		}
		fresh := reflect.New(v.Type().Elem())
		if err := gob.NewDecoder(bytes.NewReader(e.Data)).DecodeValue(fresh); err != nil {
			return true, err
		}
		v.Elem().Set(fresh.Elem())
		return true, nil
	}
	return false, nil
}
