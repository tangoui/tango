package platform

import (
	"crypto/sha256"
	"encoding/hex"
)

// WebProfileSupport reports whether the backend supports isolated stores.
type WebProfileSupport interface {
	SupportsWebProfiles() bool
}

// WebInspector controls a native inspector window.
type WebInspector interface {
	DevToolsOpen() (bool, error)
	CloseDevTools() error
}

func ValidWebProfile(s string) bool {
	if len(s) > 64 || s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// WebProfileKey maps a case-sensitive name to a filesystem-safe key.
func WebProfileKey(name string) string {
	hash := sha256.Sum256([]byte(name))
	return hex.EncodeToString(hash[:16])
}
