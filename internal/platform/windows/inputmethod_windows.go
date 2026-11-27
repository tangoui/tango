//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.InputMethods     = (*Backend)(nil)
	_ platform.InputMethodModes = (*Backend)(nil)
)

type tipConfig struct {
	Connection string
	Modes      []proto.Mode
}

func readTIPConfig() (tipConfig, error) {
	var config tipConfig
	exe, err := os.Executable()
	if err != nil {
		return config, err
	}
	data, err := os.ReadFile(exe + ".ime.json")
	if errors.Is(err, os.ErrNotExist) {
		return config, nil
	}
	if err != nil {
		return config, err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return config, err
	}
	for i := range config.Modes {
		mode := &config.Modes[i]
		if mode.Icon != "" {
			mode.Icon = filepath.Join(filepath.Dir(exe), mode.Icon)
		}
	}
	return config, nil
}

func (b *Backend) ServeInputMethod(connection string, sink platform.InputMethodSink) error {
	if b.inputMethod != nil {
		return errors.New("tango/windows: input method already serving")
	}
	config, err := readTIPConfig()
	if err != nil {
		return fmt.Errorf("tango/windows: load input method config: %w", err)
	}
	if connection == "" {
		connection = config.Connection
	}
	if connection == "" {
		connection = b.opts.DefaultIMEConnection()
	}
	if len(connection) == 0 || len(connection) > 120 || strings.ContainsFunc(connection, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_')
	}) {
		return errors.New("tango/windows: invalid input method connection name")
	}
	server, err := newTIPServer(connection, newTIPSessions(sink, config.Modes), b.runDeferred)
	if err != nil {
		return err
	}
	b.inputMethod = server
	go server.accept()
	return nil
}

func (b *Backend) IMECurrentMode() string {
	if b.inputMethod == nil {
		return ""
	}
	return b.inputMethod.current
}

func (b *Backend) IMESetMode(id string) error {
	if b.inputMethod == nil {
		return platform.ErrUnsupported
	}
	return b.inputMethod.setMode(id, false)
}

func (b *Backend) IMESetComposition(id uint64, text string, cursor int) {
	if session := b.inputMethod.byID[id]; session != nil {
		session.post(proto.Compose{Session: session.remoteID, Text: text, Cursor: uint32(max(cursor, 0))})
	}
}

func (b *Backend) IMECommit(id uint64, text string) {
	if session := b.inputMethod.byID[id]; session != nil {
		session.post(proto.Commit{Session: session.remoteID, Text: text})
	}
}

func (b *Backend) IMECaret(id uint64) platform.Rect {
	if session := b.inputMethod.byID[id]; session != nil {
		return session.caret
	}
	return platform.Rect{}
}

func (b *Backend) IMEClientApp(id uint64) string {
	if session := b.inputMethod.byID[id]; session != nil {
		return session.client.exe
	}
	return ""
}
