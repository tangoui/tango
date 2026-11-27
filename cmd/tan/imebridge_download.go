package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

//go:embed imebridge-checksums.txt
var imeBridgeSums string

var imeBridgeBaseURL = "https://github.com/tangoui/tango/releases/download"

func imeBridgeFile(ctx context.Context, cfg *Config, arch, dir string) (string, error) {
	name := "TangoIMEBridge-" + arch + ".dll"
	if local := os.Getenv("TANGO_IME_BRIDGE_DIR"); local != "" {
		file, err := filepath.Abs(filepath.Join(local, name))
		if err != nil {
			return "", err
		}
		if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
			return "", fmt.Errorf("TANGO_IME_BRIDGE_DIR has no regular %s", name)
		}
		return file, nil
	}
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-json", tangoModule)
	cmd.Dir = cfg.dir
	info, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("resolve Tango module version for IME bridge: %s", strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("resolve Tango module version for IME bridge: %w", err)
	}
	type mod struct {
		Path, Version string
		Main          bool
		Replace       *struct{ Path, Version string }
	}
	var module mod
	if err := json.Unmarshal(info, &module); err != nil {
		return "", err
	}
	version := module.Version
	if module.Replace != nil {
		if module.Replace.Path != tangoModule || module.Replace.Version == "" {
			return "", fmt.Errorf("set TANGO_IME_BRIDGE_DIR for a Tango module replacement")
		}
		version = module.Replace.Version
	}
	if module.Main || !semver.IsValid(version) || strings.Contains(version, "+") {
		return "", fmt.Errorf("set TANGO_IME_BRIDGE_DIR when building against an unreleased Tango module")
	}
	for line := range strings.Lines(imeBridgeSums) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == version && fields[1] == arch {
			return downloadVerified(ctx, http.DefaultClient, imeBridgeBaseURL+"/"+version+"/"+name, fields[2], dir)
		}
	}
	return "", fmt.Errorf("no trusted IME bridge checksum for Tango %s (%s); use a matching release of tan or set TANGO_IME_BRIDGE_DIR", version, arch)
}
