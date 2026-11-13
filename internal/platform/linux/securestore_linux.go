//go:build linux && !android

package linux

// platform.SecureStore through libsecret (the freedesktop Secret Service:
// GNOME Keyring, KWallet 5.97+, KeePassXC). Items use the schema
// org.tangoui.tango.SecureStore with the attributes "app" (the app ID)
// and "key". Values are base64 text (libsecret's password API is
// string-based). Without libsecret or a Secret Service on the session bus
// every call returns platform.ErrUnsupported.
//
// libsecret's *_sync functions run their own main context, so they are
// safe from any goroutine, as the contract allows for SecureStore.

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.SecureStore = (*backend)(nil)

// secretSchema mirrors SecretSchema (secret-schema.h) on 64-bit:
// name, flags, 32 attributes {name, type}, then reserved fields.
type secretSchema struct {
	name      *byte
	flags     int32
	_         int32
	attrs     [32]secretSchemaAttr
	reserved  int32
	_         int32
	reserved1 [7]ptr
}

type secretSchemaAttr struct {
	name *byte
	typ  int32 // SECRET_SCHEMA_ATTRIBUTE_STRING = 0
	_    int32
}

var sec struct {
	once sync.Once
	err  error

	schema *secretSchema // pinned in a global; libsecret copies static schemas

	store  func(schema *secretSchema, attrs ptr, collection, label, password *byte, cancellable ptr, gerr *ptr) bool
	lookup func(schema *secretSchema, attrs ptr, cancellable ptr, gerr *ptr) ptr
	clear  func(schema *secretSchema, attrs ptr, cancellable ptr, gerr *ptr) bool
	free   func(p ptr)

	hashNew    func(hash, equal, keyFree, valueFree ptr) ptr
	hashInsert func(h ptr, key, value ptr) bool
	hashUnref  func(h ptr)
	strdup     func(s string) ptr
	strHash    ptr
	strEqual   ptr
}

var schemaStrings [3][]byte // keeps the schema's C strings alive

func loadSecret() error {
	sec.once.Do(func() {
		if err := loadSystemLibs(); err != nil { // GLib/GIO bound by load()
			sec.err = err
			return
		}
		lib, _, err := open("libsecret-1.so.0")
		if err != nil {
			sec.err = fmt.Errorf("%w: secure storage needs libsecret (libsecret-1-0 on Debian/Ubuntu, libsecret on Fedora/Arch): %v", platform.ErrUnsupported, err)
			return
		}
		var bd binder
		bd.req(&sec.store, "secret_password_storev_sync", lib)
		bd.req(&sec.lookup, "secret_password_lookupv_sync", lib)
		bd.req(&sec.clear, "secret_password_clearv_sync", lib)
		bd.req(&sec.free, "secret_password_free", lib)
		bd.req(&sec.hashNew, "g_hash_table_new_full", libGLib)
		bd.req(&sec.hashInsert, "g_hash_table_insert", libGLib)
		bd.req(&sec.hashUnref, "g_hash_table_unref", libGLib)
		bd.req(&sec.strdup, "g_strdup", libGLib)
		sec.strHash = sym(libGLib, "g_str_hash")
		sec.strEqual = sym(libGLib, "g_str_equal")
		if len(bd.missing) > 0 || sec.strHash == 0 || sec.strEqual == 0 {
			sec.err = fmt.Errorf("%w: libsecret too old (missing %v)", platform.ErrUnsupported, bd.missing)
			return
		}
		for i, s := range []string{secretSchemaName, "app", "key"} {
			schemaStrings[i] = append([]byte(s), 0)
		}
		sec.schema = &secretSchema{name: &schemaStrings[0][0]}
		sec.schema.attrs[0] = secretSchemaAttr{name: &schemaStrings[1][0]}
		sec.schema.attrs[1] = secretSchemaAttr{name: &schemaStrings[2][0]}
	})
	return sec.err
}

// secretAttrs builds the attribute table {app, key}; the caller unrefs it.
func (b *backend) secretAttrs(key string) ptr {
	h := sec.hashNew(sec.strHash, sec.strEqual, gFreeSym, gFreeSym)
	sec.hashInsert(h, sec.strdup("app"), sec.strdup(secretApp(b.opts.ID, b.opts.Name)))
	sec.hashInsert(h, sec.strdup("key"), sec.strdup(key))
	return h
}

func secretErr(op string, gerr ptr) error {
	msg := gErrorMessage(gerr)
	if secretServiceMissing(msg) {
		return fmt.Errorf("%w: no Secret Service on the session bus (install and unlock GNOME Keyring, KWallet or KeePassXC): %s", platform.ErrUnsupported, msg)
	}
	return fmt.Errorf("tango/linux: secret %s: %s", op, msg)
}

func (b *backend) SecureSet(key string, value []byte) error {
	if err := loadSecret(); err != nil {
		return err
	}
	attrs := b.secretAttrs(key)
	defer sec.hashUnref(attrs)
	label := secretLabel(secretApp(b.opts.ID, b.opts.Name), key)
	pw := encodeSecret(value)
	var gerr ptr
	pwC := cstrAlways(pw)
	ok := sec.store(sec.schema, attrs, nil, cstrAlways(label), pwC, 0, &gerr)
	clear(unsafe.Slice(pwC, len(pw))) // do not leave the secret in the Go heap
	runtime.KeepAlive(sec.schema)
	if !ok {
		return secretErr("write", gerr)
	}
	return nil
}

func (b *backend) SecureGet(key string) ([]byte, error) {
	if err := loadSecret(); err != nil {
		return nil, err
	}
	attrs := b.secretAttrs(key)
	defer sec.hashUnref(attrs)
	var gerr ptr
	p := sec.lookup(sec.schema, attrs, 0, &gerr)
	runtime.KeepAlive(sec.schema)
	if p == 0 {
		if gerr != 0 {
			return nil, secretErr("read", gerr)
		}
		return nil, platform.ErrNotFound
	}
	s := goStr(p)
	sec.free(p) // clears the memory too
	return decodeSecret(s)
}

func (b *backend) SecureDelete(key string) error {
	if err := loadSecret(); err != nil {
		return err
	}
	attrs := b.secretAttrs(key)
	defer sec.hashUnref(attrs)
	var gerr ptr
	sec.clear(sec.schema, attrs, 0, &gerr) // FALSE without error: nothing to delete
	runtime.KeepAlive(sec.schema)
	if gerr != 0 {
		return secretErr("delete", gerr)
	}
	return nil
}

// cstrAlways is cstr that returns a pointer for "" too (non-nullable
// parameters).
func cstrAlways(s string) *byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return &b[0]
}

// sym looks a symbol up, 0 if missing.
func sym(lib ptr, name string) ptr {
	p, err := purego.Dlsym(lib, name)
	if err != nil {
		return 0
	}
	return p
}
