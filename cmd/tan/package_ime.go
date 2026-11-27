package main

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

var (
	imeGUIDPattern       = regexp.MustCompile(`^(?i)\{[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\}$`)
	imeLangPattern       = regexp.MustCompile(`^(?i)[0-9a-f]{4}$`)
	imeConnectionPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,120}$`)
)

func (c *Config) windowsIME() (IMEWindows, error) {
	result := IMEWindows{}
	if c.InputMethod != nil && c.InputMethod.Windows != nil {
		result = *c.InputMethod.Windows
	}
	if result.CLSID == "" {
		result.CLSID = imeGUID(c.ID + "/clsid")
	}
	if result.Profile == "" {
		result.Profile = imeGUID(c.ID + "/profile")
	}
	if result.LangID == "" {
		result.LangID = "0804"
	}
	for _, field := range []struct {
		name  string
		value *string
	}{{"clsid", &result.CLSID}, {"profile", &result.Profile}} {
		if len(*field.value) == 36 {
			*field.value = "{" + *field.value + "}"
		}
		if !imeGUIDPattern.MatchString(*field.value) {
			return result, fmt.Errorf("inputMethod.windows.%s must be a GUID", field.name)
		}
	}
	result.LangID = strings.TrimPrefix(strings.ToLower(result.LangID), "0x")
	if !imeLangPattern.MatchString(result.LangID) || result.LangID == "0000" {
		return result, fmt.Errorf("inputMethod.windows.langid must be a nonzero four-digit hexadecimal Windows language ID")
	}
	if !imeConnectionPattern.MatchString(c.InputMethodConnection()) {
		return result, fmt.Errorf("inputMethod.connection must be a Windows pipe name of 1–120 letters, digits, dots, hyphens or underscores")
	}
	return result, nil
}

func imeGUID(name string) string {
	// RFC 9562 UUIDv5 with Tango's fixed input method namespace.
	namespace := []byte{0x5e, 0x9c, 0x35, 0x8c, 0xb1, 0x9b, 0x45, 0xa4, 0xa3, 0xc4, 0xe0, 0x33, 0xaa, 0x4c, 0xdc, 0x1d}
	sum := sha1.Sum(append(namespace, strings.ToLower(name)...))
	sum[6] = sum[6]&0x0f | 0x50
	sum[8] = sum[8]&0x3f | 0x80
	return fmt.Sprintf("{%x-%x-%x-%x-%x}", sum[:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}

type imePackage struct {
	DLL, INI nsisHelper
	Stem     string
	Files    []nsisHelper
}

func prepareIMEPackage(ctx context.Context, cfg *Config, a artifact) (*imePackage, error) {
	if cfg.InputMethod == nil {
		return nil, nil
	}
	config, err := cfg.windowsIME()
	if err != nil {
		return nil, err
	}
	var arch string
	switch a.target.Arch {
	case "amd64":
		arch = "x64"
	case "arm64":
		arch = "arm64"
	default:
		return nil, fmt.Errorf("Windows input methods support amd64 and arm64 packages")
	}
	staging := filepath.Join(a.dir, "ime")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, err
	}
	dll, err := imeBridgeFile(ctx, cfg, arch, staging)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(dll)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(data)
	base := fmt.Sprintf("TangoIMEBridge-%s-%x-%s", cfg.Version, hash[:6], arch)
	files := &imePackage{
		DLL: nsisHelper{base + ".dll", filepath.Join(staging, base+".dll")},
		INI: nsisHelper{base + ".ini", filepath.Join(staging, base+".ini")}, Stem: base,
	}
	if err := os.WriteFile(files.DLL.Source, data, 0o644); err != nil {
		return nil, err
	}
	exe := cfg.Exe() + ".exe"
	name := strings.NewReplacer("\r", " ", "\n", " ").Replace(cfg.Name)
	ini := fmt.Sprintf("[IME]\r\nclsid=%s\r\nprofile=%s\r\nconnection=%s\r\nlangid=%s\r\nname=%s\r\nhost=%s\r\n",
		config.CLSID, config.Profile, cfg.InputMethodConnection(), config.LangID, name, exe)
	write := func(name string, contents []byte) error {
		src := filepath.Join(staging, name)
		if err := os.WriteFile(src, contents, 0o644); err != nil {
			return err
		}
		files.Files = append(files.Files, nsisHelper{name, src})
		return nil
	}
	encoded := []byte{0xff, 0xfe}
	for _, ch := range utf16.Encode([]rune(ini)) {
		encoded = binary.LittleEndian.AppendUint16(encoded, ch)
	}
	if err := os.WriteFile(files.INI.Source, encoded, 0o644); err != nil {
		return nil, err
	}
	modes := append([]InputMode(nil), cfg.InputMethod.Modes...)
	for i := range modes {
		mode := &modes[i]
		rel := mode.Icon
		mode.Icon = ""
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".ico", ".png":
			data, err := imeModeIcon(cfg.Path(rel))
			if err != nil {
				return nil, err
			}
			mode.Icon = "ime-mode-" + mode.ID + ".ico"
			if err := write(mode.Icon, data); err != nil {
				return nil, err
			}
		}
	}
	host, err := json.Marshal(struct {
		Connection string      `json:"connection"`
		CLSID      string      `json:"clsid"`
		Modes      []InputMode `json:"modes"`
	}{cfg.InputMethodConnection(), config.CLSID, modes})
	if err != nil {
		return nil, err
	}
	if err := write(exe+".ime.json", host); err != nil {
		return nil, err
	}
	return files, nil
}

func imeModeIcon(path string) ([]byte, error) {
	if strings.EqualFold(filepath.Ext(path), ".ico") {
		return os.ReadFile(path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	image, _, err := image.Decode(file)
	if err != nil {
		return nil, err
	}
	frames, err := pngFrames(image)
	if err != nil {
		return nil, err
	}
	return icoFile(frames), nil
}
