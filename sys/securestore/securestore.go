// Package securestore keeps small secrets (tokens, keys, passwords) in the
// platform's protected storage: the Keychain on iOS and macOS (generic
// passwords of the app, this device only, available after first unlock);
// on Android values encrypted with an AES key held in the Android
// Keystore, stored in the app's private SharedPreferences; the Credential
// Manager on Windows (generic credentials of the user, not roaming); and
// the Secret Service on Linux (GNOME Keyring, KWallet) through libsecret.
// Linux returns ErrUnsupported when libsecret-1 is not installed or no
// Secret Service is running (headless sessions).
//
//	securestore.SetString(a, "session", tok)
//	tok, err := securestore.GetString(a, "session") // ErrNotFound if never set
//
// Items survive app updates; on iOS Keychain items may survive
// uninstalling the app. Values are meant to be small (kilobytes). Calls are
// safe from any goroutine and do not need the main thread. They return
// ErrUnsupported where the backend has no secure store.
package securestore

import (
	"errors"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/sys/internal/capcall"
)

var (
	ErrUnsupported = platform.ErrUnsupported
	ErrNotFound    = platform.ErrNotFound
)

var errKey = errors.New("tango/securestore: empty key")

// Set stores value under key, replacing any previous value.
func Set(a *tango.App, key string, value []byte) error {
	s, err := capcall.Backend[platform.SecureStore](a)
	if err != nil {
		return err
	}
	if key == "" {
		return errKey
	}
	if value == nil {
		value = []byte{}
	}
	return s.SecureSet(key, value)
}

// Get returns the value stored under key, or ErrNotFound.
func Get(a *tango.App, key string) ([]byte, error) {
	s, err := capcall.Backend[platform.SecureStore](a)
	if err != nil {
		return nil, err
	}
	if key == "" {
		return nil, errKey
	}
	return s.SecureGet(key)
}

// Delete removes key; deleting a missing key is not an error.
func Delete(a *tango.App, key string) error {
	s, err := capcall.Backend[platform.SecureStore](a)
	if err != nil {
		return err
	}
	if key == "" {
		return errKey
	}
	return s.SecureDelete(key)
}

// SetString is Set for text.
func SetString(a *tango.App, key, value string) error { return Set(a, key, []byte(value)) }

// GetString is Get for text.
func GetString(a *tango.App, key string) (string, error) {
	b, err := Get(a, key)
	return string(b), err
}
