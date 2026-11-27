package main

import (
	"context"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/mod/semver"
)

//go:embed imebridge-checksums.txt
var imeBridgeSums string

var (
	imeBridgeBaseURL    = "https://github.com/tangoui/tango/releases/download"
	imeBridgeReleaseAPI = "https://api.github.com/repos/tangoui/tango/releases/tags"
)

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
	sum, err := imeBridgeChecksum(ctx, version, arch, name)
	if err != nil {
		return "", err
	}
	return downloadVerified(ctx, http.DefaultClient, imeBridgeBaseURL+"/"+version+"/"+name, sum, dir)
}

func imeBridgeChecksum(ctx context.Context, version, arch, name string) (string, error) {
	for line := range strings.Lines(imeBridgeSums) {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == version && fields[1] == arch {
			return fields[2], nil
		}
	}
	// Source-installed tan has no CI-generated table. Resolve the asset's
	// SHA-256 from the official repository's HTTPS release metadata.
	ctx, cancel := context.WithTimeout(ctx, nsisDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imeBridgeReleaseAPI+"/"+version, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("resolve trusted Tango %s IME bridge checksum: %s", version, resp.Status)
	}
	var release struct {
		Tag    string `json:"tag_name"`
		Assets []struct{ Name, Digest string }
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&release); err != nil {
		return "", err
	}
	if release.Tag == version {
		for _, asset := range release.Assets {
			digest, ok := strings.CutPrefix(asset.Digest, "sha256:")
			decoded, err := hex.DecodeString(digest)
			if asset.Name == name && ok && err == nil && len(decoded) == 32 {
				return digest, nil
			}
		}
	}
	return "", fmt.Errorf("no trusted IME bridge checksum for Tango %s (%s); set TANGO_IME_BRIDGE_DIR for a local bridge", version, arch)
}
