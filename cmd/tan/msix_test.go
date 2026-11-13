package main

import (
	"context"
	"encoding/xml"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appxPackage decodes the parts of AppxManifest.xml the tests check.
type appxPackage struct {
	XMLName  xml.Name `xml:"http://schemas.microsoft.com/appx/manifest/foundation/windows10 Package"`
	Identity struct {
		Name, Publisher, Version, ProcessorArchitecture string `xml:",attr"`
	} `xml:"Identity"`
	Properties struct {
		DisplayName, PublisherDisplayName, Logo string
	} `xml:"Properties"`
	Family struct {
		Name, MinVersion string `xml:",attr"`
	} `xml:"Dependencies>TargetDeviceFamily"`
	App struct {
		ID         string `xml:"Id,attr"`
		Executable string `xml:",attr"`
		EntryPoint string `xml:",attr"`
		Visual     struct {
			DisplayName       string `xml:",attr"`
			Square150x150Logo string `xml:",attr"`
			Square44x44Logo   string `xml:",attr"`
		} `xml:"http://schemas.microsoft.com/appx/manifest/uap/windows10 VisualElements"`
		Extensions []struct {
			Category string `xml:",attr"`
			Protocol struct {
				Name string `xml:",attr"`
			} `xml:"http://schemas.microsoft.com/appx/manifest/uap/windows10 Protocol"`
			FileTypes []string `xml:"http://schemas.microsoft.com/appx/manifest/uap/windows10 FileTypeAssociation>SupportedFileTypes>FileType"`
		} `xml:"Extensions>Extension"`
	} `xml:"Applications>Application"`
	Capabilities []struct {
		XMLName xml.Name
		Name    string `xml:",attr"`
	} `xml:"Capabilities>Capability"`
}

func msixConfig(t *testing.T) *Config {
	cfg := assocConfig(t)
	cfg.Name = "Hello & Co"
	cfg.Version = "1.4.2-beta.3"
	cfg.Windows.Publisher = `CN=Example Ltd, O=Example Ltd, C=US`
	return cfg
}

func TestAppxManifest(t *testing.T) {
	cfg := msixConfig(t)
	for arch, want := range map[string]string{"amd64": "x64", "arm64": "arm64"} {
		var p appxPackage
		if err := xml.Unmarshal(appxManifest(cfg, arch), &p); err != nil {
			t.Fatalf("%s: %v\n%s", arch, err, appxManifest(cfg, arch))
		}
		id := p.Identity
		if id.Name != "com.example.hello" || id.Publisher != cfg.Windows.Publisher || id.Version != "1.4.2.0" || id.ProcessorArchitecture != want {
			t.Errorf("%s: identity %+v", arch, id)
		}
		if p.Properties.DisplayName != "Hello & Co" || p.Properties.PublisherDisplayName != "Example Ltd" || p.Properties.Logo != `Assets\StoreLogo.png` {
			t.Errorf("properties %+v", p.Properties)
		}
		if p.Family.Name != "Windows.Desktop" || p.Family.MinVersion != "10.0.17763.0" {
			t.Errorf("device family %+v", p.Family)
		}
		if p.App.ID != "App" || p.App.Executable != "HelloCo.exe" || p.App.EntryPoint != "Windows.FullTrustApplication" {
			t.Errorf("application %+v", p.App)
		}
		if p.App.Visual.DisplayName != "Hello & Co" || p.App.Visual.Square150x150Logo != `Assets\Square150x150Logo.png` || p.App.Visual.Square44x44Logo != `Assets\Square44x44Logo.png` {
			t.Errorf("visual elements %+v", p.App.Visual)
		}
		var cats []string
		for _, e := range p.App.Extensions {
			cats = append(cats, e.Category+":"+e.Protocol.Name+strings.Join(e.FileTypes, ","))
		}
		if strings.Join(cats, " ") != "windows.protocol:hello windows.protocol:hello-dev windows.fileTypeAssociation:.hlo windows.fileTypeAssociation:.md" {
			t.Errorf("extensions %v", cats)
		}
		if len(p.Capabilities) != 1 || p.Capabilities[0].Name != "runFullTrust" ||
			p.Capabilities[0].XMLName.Space != "http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities" {
			t.Errorf("capabilities %+v", p.Capabilities)
		}
	}
	plain := testConfig(t)
	plain.Windows.Publisher = "CN=X"
	if strings.Contains(string(appxManifest(plain, "amd64")), "<Extensions>") {
		t.Error("extensions without schemes or associations")
	}
}

func TestMSIXHelpers(t *testing.T) {
	for in, want := range map[string]string{"1.2.3": "1.2.3.0", "2.0.0-rc.1+b5": "2.0.0.0", "3": "3.0.0.0", "1.70000.0": "1.65535.0.0"} {
		if got := msixVersion(in); got != want {
			t.Errorf("msixVersion(%q) = %q, want %q", in, got, want)
		}
	}
	if got := publisherDisplayName(`O=Org, CN="Jane Doe", C=US`); got != "Jane Doe" {
		t.Errorf("publisher display name %q", got)
	}
	cfg := testConfig(t)
	win := []target{{"windows", "amd64"}}
	if err := checkPackageFormat(cfg, "", []target{{"linux", "amd64"}}); err != nil {
		t.Errorf("default format: %v", err)
	}
	if err := checkPackageFormat(cfg, "msix", win); err == nil || !strings.Contains(err.Error(), "windows.publisher") {
		t.Errorf("no publisher: %v", err)
	}
	cfg.Windows.Publisher = "CN=X"
	if err := checkPackageFormat(cfg, "msix", win); err != nil {
		t.Errorf("msix: %v", err)
	}
	if err := checkPackageFormat(cfg, "msix", []target{{"windows", "arm64"}, {"darwin", "arm64"}}); err == nil {
		t.Error("msix for darwin accepted")
	}
	if err := checkPackageFormat(cfg, "zip", win); err == nil {
		t.Error("unknown format accepted")
	}
}

func TestPackageMSIXLayout(t *testing.T) {
	cfg := msixConfig(t)
	cfg.Icon = "icon.png"
	os.WriteFile(cfg.Path("icon.png"), defaultIcon("x"), 0o644)
	t.Setenv("PATH", t.TempDir()) // no makeappx, no signing tools
	t.Setenv(envWindowsCert, "")
	a := artifact{target: target{"windows", "arm64"}, dir: filepath.Join(cfg.dir, "build", "windows-arm64")}
	os.MkdirAll(a.dir, 0o755)
	a.exe = filepath.Join(a.dir, "HelloCo.exe")
	os.WriteFile(a.exe, []byte("MZ"), 0o755)
	made, err := packageMSIX(context.Background(), cfg, a)
	if err != nil {
		t.Fatal(err)
	}
	layout := filepath.Join(cfg.dir, "build", "HelloCo-1.4.2-beta.3-windows-arm64-msix")
	if len(made) != 1 || made[0] != layout {
		t.Fatalf("made %v", made)
	}
	if b, _ := os.ReadFile(filepath.Join(layout, "HelloCo.exe")); string(b) != "MZ" {
		t.Errorf("exe %q", b)
	}
	var p appxPackage
	b, _ := os.ReadFile(filepath.Join(layout, "AppxManifest.xml"))
	if err := xml.Unmarshal(b, &p); err != nil || p.Identity.ProcessorArchitecture != "arm64" {
		t.Errorf("manifest %v %+v", err, p.Identity)
	}
	for _, as := range msixAssets {
		f, err := os.Open(filepath.Join(layout, "Assets", as.name))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", as.name, err)
		}
		if bd := img.Bounds(); bd.Dx() != as.size || bd.Dy() != as.size {
			t.Errorf("%s is %v, want %d", as.name, bd, as.size)
		}
	}
	// Without an icon the default one fills in.
	cfg.Icon = ""
	if err := writeMSIXLayout(cfg, a, layout); err != nil {
		t.Fatal(err)
	}
}
