package main

// MSIX packaging: tan package -format msix lays out a package directory
// (the executable, AppxManifest.xml, Assets) and, when the Windows SDK's
// makeappx is on PATH, packs and signs it. The app runs as a full-trust
// desktop app (Windows.FullTrustApplication + runFullTrust).

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// msixMinVersion is Windows 10 1809, the first with MSIX.
const msixMinVersion = "10.0.17763.0"

// msixAssets are the logos the manifest names, with their pixel sizes.
var msixAssets = []struct {
	name string
	size int
}{
	{"StoreLogo.png", 50},
	{"Square44x44Logo.png", 44},
	{"Square150x150Logo.png", 150},
}

// checkPackageFormat validates tan package -format before anything is built.
func checkPackageFormat(cfg *Config, format string, ts []target) error {
	switch format {
	case "":
		return nil
	case "msix":
	default:
		return checkLinuxFormats(format, ts) // linux_pkg.go
	}
	for _, t := range ts {
		if t.OS != "windows" {
			return fmt.Errorf("-format msix is for Windows targets, not %s", t)
		}
	}
	if cfg.Windows.Publisher == "" {
		return fmt.Errorf(`MSIX needs "windows.publisher" in %s: the subject of the signing certificate, like "CN=Example Ltd, O=Example Ltd, C=US"`, ConfigFile)
	}
	if n := len(cfg.ID); n < 3 || n > 50 {
		return fmt.Errorf(`MSIX package names are 3 to 50 characters; "id" %q has %d`, cfg.ID, n)
	}
	return nil
}

// msixVersion is the package version: x.y.z.0, each part at most 65535,
// a pre-release or build suffix dropped.
func msixVersion(v string) string {
	v, _, _ = strings.Cut(v, "-")
	v, _, _ = strings.Cut(v, "+")
	var parts [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			break
		}
		parts[i] = min(n, 65535)
	}
	return fmt.Sprintf("%d.%d.%d.0", parts[0], parts[1], parts[2])
}

// msixArch maps GOARCH to ProcessorArchitecture.
func msixArch(goarch string) string {
	if goarch == "amd64" {
		return "x64"
	}
	return goarch // arm64
}

// publisherDisplayName is the CN of a certificate subject.
func publisherDisplayName(subject string) string {
	for _, part := range strings.Split(subject, ",") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(part), "CN="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return subject
}

// appxManifest renders AppxManifest.xml. URL schemes and file
// associations from tan.json become protocol and file-type extensions.
func appxManifest(cfg *Config, arch string) []byte {
	x := html.EscapeString
	exe := cfg.Exe() + ".exe"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
         xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
         xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
         IgnorableNamespaces="uap rescap">
`)
	fmt.Fprintf(&b, "  <Identity Name=\"%s\" Publisher=\"%s\" Version=\"%s\" ProcessorArchitecture=\"%s\"/>\n",
		x(cfg.ID), x(cfg.Windows.Publisher), msixVersion(cfg.Version), msixArch(arch))
	fmt.Fprintf(&b, `  <Properties>
    <DisplayName>%s</DisplayName>
    <PublisherDisplayName>%s</PublisherDisplayName>
    <Logo>Assets\StoreLogo.png</Logo>
  </Properties>
  <Dependencies>
    <TargetDeviceFamily Name="Windows.Desktop" MinVersion="%s" MaxVersionTested="10.0.26100.0"/>
  </Dependencies>
  <Resources>
    <Resource Language="en-us"/>
  </Resources>
  <Applications>
    <Application Id="App" Executable="%s" EntryPoint="Windows.FullTrustApplication">
      <uap:VisualElements DisplayName="%s" Description="%s" BackgroundColor="transparent" Square150x150Logo="Assets\Square150x150Logo.png" Square44x44Logo="Assets\Square44x44Logo.png"/>
`, x(cfg.Name), x(publisherDisplayName(cfg.Windows.Publisher)), msixMinVersion, x(exe), x(cfg.Name), x(cfg.Name))
	if len(cfg.URLSchemes)+len(cfg.FileAssociations) > 0 {
		b.WriteString("      <Extensions>\n")
		for _, sc := range cfg.URLSchemes {
			fmt.Fprintf(&b, "        <uap:Extension Category=\"windows.protocol\">\n          <uap:Protocol Name=\"%s\"/>\n        </uap:Extension>\n", x(sc))
		}
		for _, f := range cfg.FileAssociations {
			ext := strings.ToLower(f.Ext)
			fmt.Fprintf(&b, "        <uap:Extension Category=\"windows.fileTypeAssociation\">\n          <uap:FileTypeAssociation Name=\"%s\">\n            <uap:DisplayName>%s</uap:DisplayName>\n            <uap:SupportedFileTypes>\n              <uap:FileType>.%s</uap:FileType>\n            </uap:SupportedFileTypes>\n          </uap:FileTypeAssociation>\n        </uap:Extension>\n",
				x(ext), x(f.Name), x(ext))
		}
		b.WriteString("      </Extensions>\n")
	}
	b.WriteString(`    </Application>
  </Applications>
  <Capabilities>
    <rescap:Capability Name="runFullTrust"/>
  </Capabilities>
</Package>
`)
	return []byte(b.String())
}

// writeMSIXLayout writes the package directory: the executable, the
// manifest and the logos (scaled from tan.json's icon, or tan's default
// icon when there is none).
func writeMSIXLayout(cfg *Config, a artifact, layout string) error {
	if err := os.RemoveAll(layout); err != nil {
		return err
	}
	assets := filepath.Join(layout, "Assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		return err
	}
	if err := copyFile(a.exe, filepath.Join(layout, cfg.Exe()+".exe"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(layout, "AppxManifest.xml"), appxManifest(cfg, a.target.Arch), 0o644); err != nil {
		return err
	}
	src, err := msixLogo(cfg)
	if err != nil {
		return fmt.Errorf("MSIX logo: %w", err)
	}
	for _, as := range msixAssets {
		b, err := pngBytes(scaleIcon(src, as.size)) // icon_desktop.go
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(assets, as.name), b, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// msixLogo is the Windows desktop icon (icon_desktop.go): "icons.windows"
// when it is a PNG, else the shape derived from the master icon, else the
// derived shape of tan's default icon.
func msixLogo(cfg *Config) (image.Image, error) {
	override := cfg.Icons.Windows
	if strings.EqualFold(filepath.Ext(override), ".ico") {
		override = ""
	}
	img, err := desktopIconImage(cfg, "windows", override)
	if err != nil || img != nil {
		return img, err
	}
	master, err := png.Decode(bytes.NewReader(defaultIcon(cfg.Name))) // init.go
	if err != nil {
		return nil, err
	}
	return desktopIcon(master), nil
}

// packageMSIX makes <base>-msix (the layout) and, with makeappx, <base>.msix,
// signed when a certificate is configured.
func packageMSIX(ctx context.Context, cfg *Config, a artifact) ([]string, error) {
	signer, sign, err := packageSigner(cfg)
	if err != nil {
		return nil, err
	}
	if sign {
		if err := signWindowsFile(ctx, signer, a.exe); err != nil {
			return nil, err
		}
	}
	base := filepath.Join(filepath.Dir(a.dir), artifactBase(cfg, a.target))
	layout := base + "-msix"
	if err := writeMSIXLayout(cfg, a, layout); err != nil {
		return nil, err
	}
	made := []string{layout}
	tool, err := exec.LookPath("makeappx")
	if err != nil {
		warnf("makeappx is not on PATH: wrote the package layout %s; pack it on Windows with: makeappx pack /d <layout> /p <name>.msix, then tan sign it", filepath.Base(layout))
		return made, nil
	}
	msix := base + ".msix"
	if err := runIn(ctx, "", nil, tool, "pack", "/d", layout, "/p", msix, "/o"); err != nil {
		return made, err
	}
	made = append(made, msix)
	if sign {
		if err := signWindowsFile(ctx, signer, msix); err != nil {
			return made, err
		}
	} else {
		warnf("%s is unsigned: Windows installs only signed MSIX packages", filepath.Base(msix))
	}
	return made, nil
}
