package linux

import (
	"errors"
	"fmt"
	"path/filepath"
	"unicode/utf8"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform/imesession"
)

const (
	fcitxSocketMode    = 0o600
	fcitxSocketDirMode = 0o700
)

func fcitxSocketPath(runtime, connection string) (string, error) {
	if !filepath.IsAbs(runtime) {
		return "", errors.New("XDG_RUNTIME_DIR must name an absolute directory")
	}
	if !linuxIMEID.MatchString(connection) || len(connection) > 120 {
		return "", fmt.Errorf("invalid Fcitx5 connection name %q", connection)
	}
	path := filepath.Join(runtime, "tango-ime", connection+".sock")
	if len(path) > 107 {
		return "", errors.New("Fcitx5 socket path exceeds the Linux limit of 107 bytes")
	}
	return path, nil
}

func fcitxPeerAllowed(local, peer uint32) bool { return local == peer }

func fcitxKey(m *imesession.Mux, session *imesession.Session, key proto.Key) bool {
	direction := key.Flags & (proto.KeyUp | proto.KeyDown)
	if direction != proto.KeyDown && direction != proto.KeyUp {
		return false
	}
	var state uint32
	if key.Mods&proto.ModShift != 0 {
		state |= 1
	}
	if key.Mods&proto.ModCapsLock != 0 {
		state |= 2
	}
	if key.Mods&proto.ModCtrl != 0 {
		state |= 4
	}
	if key.Mods&proto.ModAlt != 0 {
		state |= 8
	}
	if key.Mods&proto.ModMeta != 0 {
		state |= 1 << 6
	}
	if direction == proto.KeyUp {
		state |= ibusReleaseMask
	}
	if mod, ok := ibusModifier(uint32(key.VK), state); ok {
		return key.Flags&proto.KeyRepeat == 0 && m.Modifier(session, mod)
	}
	if direction == proto.KeyUp {
		return false
	}
	char, _ := utf8.DecodeRuneInString(key.Text)
	if key.Text == "" {
		char = 0
	}
	event := ibusKey(uint32(key.VK), uint32(key.Scan), state, char)
	event.Repeat = key.Flags&proto.KeyRepeat != 0
	return m.Key(session, event)
}
