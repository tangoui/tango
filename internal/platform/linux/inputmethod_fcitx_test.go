package linux

import (
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/internal/imebridge/proto"
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

func TestFcitxCandidateListPeerCompatibility(t *testing.T) {
	msg := proto.Message{Body: proto.Candidates{
		Session: 1, Revision: 2, Selected: -1, Items: []proto.Candidate{{Text: "你"}},
	}}
	for _, tc := range []struct {
		exe  string
		kind proto.Type
	}{
		{"fcitx5", proto.TypeCandidates},
		{fcitxCandidateSelectionHello, proto.TypeCandidateList},
	} {
		data, err := proto.Marshal(fcitxMessageForPeer(msg, proto.Hello{Exe: tc.exe}))
		if err != nil {
			t.Fatal(err)
		}
		if got := proto.Type(binary.LittleEndian.Uint16(data[4:6])); got != tc.kind {
			t.Errorf("Hello %q got candidate frame %d, want %d", tc.exe, got, tc.kind)
		}
	}
}
