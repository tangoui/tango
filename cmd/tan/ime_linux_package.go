package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/template"
)

var linuxIMENamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func (c *Config) linuxIMEEngines() (ibus, fcitx string) {
	ibus, fcitx = c.ID, c.ID
	if cfg := c.InputMethod.Linux; cfg != nil {
		if cfg.Engine != "" {
			ibus = cfg.Engine
		}
		if cfg.IBus != "" {
			ibus = cfg.IBus
		}
		if cfg.Fcitx5 != "" {
			fcitx = cfg.Fcitx5
		}
	}
	return ibus, fcitx
}

func ibusComponentXML(cfg *Config, name, icon string) ([]byte, error) {
	type engine struct {
		Name     string `xml:"name"`
		LongName string `xml:"longname"`
		Language string `xml:"language"`
		Layout   string `xml:"layout"`
		Icon     string `xml:"icon"`
	}
	type component struct {
		XMLName     xml.Name `xml:"component"`
		Name        string   `xml:"name"`
		Description string   `xml:"description"`
		Exec        string   `xml:"exec"`
		Version     string   `xml:"version"`
		Engines     []engine `xml:"engines>engine"`
	}
	hash := sha256.Sum256([]byte(name))
	content, err := xml.MarshalIndent(component{
		Name:        fmt.Sprintf("org.freedesktop.IBus.Tango.Engine%x", hash[:8]),
		Description: cfg.Name, Exec: "/usr/bin/" + cfg.Exe() + " --ibus", Version: cfg.Version,
		Engines: []engine{{Name: name, LongName: cfg.Name, Language: linuxIMELanguage(cfg), Layout: "default", Icon: icon}},
	}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(append([]byte(xml.Header), content...), '\n'), nil
}

func linuxIMELanguage(cfg *Config) string {
	switch language := cfg.InputMethod.Language; language {
	case "", "zh", "zh-Hans", "zh-CN":
		return "zh_CN"
	case "zh-Hant", "zh-TW":
		return "zh_TW"
	default:
		return strings.ReplaceAll(language, "-", "_")
	}
}

type linuxIMEPackage struct {
	files              []sysFile
	minFcitx, maxFcitx string
}

func prepareLinuxIME(ctx context.Context, cfg *Config, a artifact, warn func(string, ...any)) (*linuxIMEPackage, error) {
	if cfg.InputMethod == nil {
		return nil, nil
	}
	ibus, fcitx := cfg.linuxIMEEngines()
	connection := cfg.InputMethodConnection()
	if !linuxIMENamePattern.MatchString(ibus) || !linuxIMENamePattern.MatchString(fcitx) ||
		!linuxIMENamePattern.MatchString(connection) || len(connection) > 120 {
		return nil, fmt.Errorf("invalid Linux input method engine or connection name")
	}
	parent := "/usr/share/" + cfg.ID
	icon := "input-keyboard"
	pkg := &linuxIMEPackage{}
	if file := cfg.InputMethod.Icon; file != "" {
		data, err := os.ReadFile(cfg.Path(file))
		if err != nil {
			return nil, err
		}
		icon = parent + "/tango-ime" + filepath.Ext(file)
		pkg.files = append(pkg.files, sysFile{path: icon, data: data, mode: 0o644})
	}
	xml, err := ibusComponentXML(cfg, ibus, icon)
	if err != nil {
		return nil, err
	}
	config, err := json.Marshal(struct {
		Connection string      `json:"connection"`
		IBusEngine string      `json:"ibusEngine"`
		Modes      []InputMode `json:"modes"`
	}{connection, ibus, cfg.InputMethod.Modes})
	if err != nil {
		return nil, err
	}
	bridge := fmt.Sprintf("host=/usr/bin/%s\nconnection=%s\nsocket=tango-ime/%s.sock\n", cfg.Exe(), connection, connection)
	pkg.files = append(pkg.files,
		sysFile{path: "/usr/share/ibus/component/" + cfg.ID + ".xml", data: xml, mode: 0o644},
		sysFile{path: parent + "/tango-ime.json", data: config, mode: 0o644},
		sysFile{path: parent + "/tango-ime.conf", data: []byte(bridge), mode: 0o644},
	)
	if runtime.GOOS != "linux" || runtime.GOARCH != a.target.Arch {
		warn("Skipping Fcitx5 addon because it requires a Linux packaging host matching %s", a.target.Arch)
		return pkg, nil
	}
	cmake, err := exec.LookPath("cmake")
	if err != nil {
		warn("Skipping Fcitx5 addon because cmake is not installed")
		return pkg, nil
	}
	version, err := exec.CommandContext(ctx, "pkg-config", "--modversion", "Fcitx5Core").Output()
	if err != nil {
		warn("Skipping Fcitx5 addon because the Fcitx5Core development package is missing")
		return pkg, nil
	}
	pkg.minFcitx, pkg.maxFcitx, err = fcitxVersionRange(strings.TrimSpace(string(version)))
	if err != nil {
		return nil, err
	}
	libdir, err := exec.CommandContext(ctx, "pkg-config", "--variable=libdir", "Fcitx5Core").Output()
	if err != nil {
		return nil, fmt.Errorf("find Fcitx5 library directory: %w", err)
	}
	lib := path.Clean(strings.TrimSpace(string(libdir)))
	if lib != "/usr/lib" && lib != "/usr/lib64" && !strings.HasPrefix(lib, "/usr/lib/") {
		return nil, fmt.Errorf("Fcitx5 is installed outside the distribution library directory: %q", lib)
	}
	module, _, err := locateTango(ctx, cfg.dir)
	if err != nil {
		return nil, err
	}
	src := filepath.Join(module, "internal", "imebridge", "fcitx5")
	dest := filepath.Join(a.dir, "fcitx5-addon")
	addon := cfg.ID
	if err := runIn(ctx, cfg.dir, nil, cmake, "-S", src, "-B", dest, "-DCMAKE_BUILD_TYPE=Release",
		"-DTANGO_FCITX_ADDON="+addon, "-DTANGO_FCITX_CONFIG="+parent+"/tango-ime.conf"); err != nil {
		return nil, fmt.Errorf("configure Fcitx5 addon: %w", err)
	}
	if err := runIn(ctx, cfg.dir, nil, cmake, "--build", dest, "--config", "Release"); err != nil {
		return nil, fmt.Errorf("build Fcitx5 addon: %w", err)
	}
	so, err := os.ReadFile(filepath.Join(dest, addon+".so"))
	if err != nil {
		return nil, err
	}
	pkg.files = append(pkg.files, sysFile{path: path.Join(lib, "fcitx5", addon+".so"), data: so, mode: 0o755})
	label := "中"
	if len(cfg.InputMethod.Modes) != 0 {
		label = cfg.InputMethod.Modes[0].Short
	}
	vars := struct{ Name, Library, LangCode, Icon, Label string }{
		desktopValue(cfg.Name), addon, desktopValue(linuxIMELanguage(cfg)), desktopValue(icon), desktopValue(label),
	}
	for _, f := range []struct{ source, target string }{
		{"addon.conf.in", "/usr/share/fcitx5/addon/" + addon + ".conf"},
		{"inputmethod.conf.in", "/usr/share/fcitx5/inputmethod/" + fcitx + ".conf"},
	} {
		tmpl, err := template.ParseFiles(filepath.Join(src, f.source))
		if err != nil {
			return nil, err
		}
		var b bytes.Buffer
		if err := tmpl.Execute(&b, vars); err != nil {
			return nil, err
		}
		pkg.files = append(pkg.files, sysFile{path: f.target, data: b.Bytes(), mode: 0o644})
	}
	return pkg, nil
}

func fcitxVersionRange(version string) (min, max string, err error) {
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		major, majorErr := strconv.Atoi(parts[0])
		minor, minorErr := strconv.Atoi(parts[1])
		if majorErr == nil && minorErr == nil && major > 0 && minor >= 0 && minor < 999 {
			return fmt.Sprintf("%d.%d", major, minor), fmt.Sprintf("%d.%d", major, minor+1), nil
		}
	}
	return "", "", fmt.Errorf("cannot determine the Fcitx5 ABI from version %q", version)
}
