package linux

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/tangoui/tango/internal/imebridge/proto"
)

type linuxIMEConfig struct {
	Connection string
	IBusEngine string
	Modes      []proto.Mode
}

var linuxIMEID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func readLinuxIMEConfig(id string) (linuxIMEConfig, error) {
	var cfg linuxIMEConfig
	if !linuxIMEID.MatchString(id) {
		return cfg, nil
	}
	data, err := os.ReadFile(filepath.Join("/usr/share", id, "tango-ime.json"))
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(data, &cfg)
}

func ibusComponentName(engine string) string {
	hash := sha256.Sum256([]byte(engine))
	return fmt.Sprintf("org.freedesktop.IBus.Tango.Engine%x", hash[:8])
}
