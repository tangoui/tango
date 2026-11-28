package linux

import (
	"path/filepath"
	"testing"
)

func TestFcitxSocketPath(t *testing.T) {
	const root = "/run/user/1000"
	got, err := fcitxSocketPath(root, "app_Connection")
	if err != nil || got != filepath.Join(root, "tango-ime", "app_Connection.sock") {
		t.Fatalf("socket = %q, %v", got, err)
	}
	if fcitxSocketMode != 0o600 || fcitxSocketDirMode != 0o700 {
		t.Fatal("socket does not use private permissions")
	}
	for _, tc := range [][2]string{{"", "app"}, {"relative", "app"}, {root, "../escape"}} {
		if path, err := fcitxSocketPath(tc[0], tc[1]); err == nil {
			t.Errorf("accepted %q with connection %q: %q", tc[0], tc[1], path)
		}
	}
}

func TestFcitxPeerCred(t *testing.T) {
	if !fcitxPeerAllowed(1000, 1000) || fcitxPeerAllowed(1000, 1001) || fcitxPeerAllowed(1000, 0) {
		t.Fatal("unexpected peer UID policy")
	}
}
