package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestIBusComponentXML(t *testing.T) {
	cfg := testConfig(t)
	cfg.ID, cfg.InputMethod = "com.example.inputmethod.hello", &InputMethod{}
	content, err := ibusComponentXML(cfg, cfg.ID, "input-keyboard")
	if err != nil {
		t.Fatal(err)
	}
	var component struct {
		Exec   string                                                  `xml:"exec"`
		Engine struct{ Name, LongName, Language, Layout, Icon string } `xml:"engines>engine"`
	}
	if err := xml.Unmarshal(content, &component); err != nil {
		t.Fatal(err)
	}
	if component.Exec != "/usr/bin/HelloApp --ibus" {
		t.Fatalf("exec = %q", component.Exec)
	}
	for _, want := range []string{"<name>" + cfg.ID + "</name>", "<longname>" + cfg.Name + "</longname>",
		"<language>zh_CN</language>", "<layout>default</layout>", "<icon>input-keyboard</icon>",
		"<author>Tango</author>", "<license>Proprietary</license>", "<homepage>https://github.com/tangoui/tango</homepage>", "<textdomain>" + cfg.ID + "</textdomain>"} {
		if !bytes.Contains(content, []byte(want)) {
			t.Errorf("IBus XML lacks %s", want)
		}
	}
	if xmllint, err := exec.LookPath("xmllint"); err == nil {
		cmd := exec.Command(xmllint, "--noout", "-")
		cmd.Stdin = bytes.NewReader(content)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("xmllint: %v, %s", err, out)
		}
	}
}

func linuxIMEFixture(t *testing.T) (*Config, artifact) {
	t.Helper()
	cfg, a := linuxPkgFixture(t)
	cfg.ID = "com.example.inputmethod.hello"
	cfg.InputMethod = &InputMethod{Modes: []InputMode{{ID: "zh", Label: "中文", Short: "中"}}}
	return cfg, a
}

func TestDebIMEFiles(t *testing.T) {
	cfg, a := linuxIMEFixture(t)
	var warnings []string
	var err error
	a.linuxIME, err = prepareLinuxIME(context.Background(), cfg, a, func(format string, args ...any) {
		warnings = append(warnings, format)
	})
	if err != nil {
		t.Fatal(err)
	}
	p, err := packageDeb(cfg, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	_, members := readAr(t, blob)
	control, _, _ := readTarGz(t, members[1])
	files, _, _ := readTarGz(t, members[2])
	checkIMEPaths(t, cfg, files)
	if !strings.Contains(string(control["./control"]), "ibus | fcitx5") ||
		!strings.Contains(strings.Join(warnings, "\n"), "Fcitx5") {
		t.Fatalf("control = %s, warnings = %v", control["./control"], warnings)
	}
	for _, name := range []string{"./postinst", "./postrm"} {
		if script := string(control[name]); !strings.Contains(script, "ibus write-cache") ||
			!strings.Contains(script, "Restart IBus or Fcitx5") {
			t.Errorf("%s = %q", name, script)
		}
	}
}

func TestRpmIMEFiles(t *testing.T) {
	cfg, a := linuxIMEFixture(t)
	made, err := packageLinuxFormats(context.Background(), cfg, a, []string{"rpm"})
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(made[0])
	if err != nil {
		t.Fatal(err)
	}
	_, count := readRPMHeader(t, blob[96:], rpmTagHeaderSignatures)
	start := 96 + (count+7)&^7
	header, size := readRPMHeader(t, blob[start:], rpmTagHeaderImmutable)
	if !slices.Contains(header.strs(rpmTagRequireName), "(ibus or fcitx5)") {
		t.Errorf("RPM dependencies: %v", header.strs(rpmTagRequireName))
	}
	reader, err := gzip.NewReader(bytes.NewReader(blob[start+size:]))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	cpio, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := readCpio(t, cpio)
	checkIMEPaths(t, cfg, files)
	for _, tag := range []uint32{rpmTagPostIn, rpmTagPostUn} {
		if !strings.Contains(strings.Join(header.strs(tag), "\n"), "ibus write-cache") {
			t.Errorf("RPM scriptlet %d lacks IBus cache update", tag)
		}
	}
}

func checkIMEPaths(t *testing.T, cfg *Config, files map[string][]byte) {
	t.Helper()
	for _, name := range []string{"./usr/share/ibus/component/" + cfg.ID + ".xml",
		"./usr/share/" + cfg.ID + "/tango-ime.conf", "./usr/share/" + cfg.ID + "/tango-ime.json"} {
		if len(files[name]) == 0 {
			t.Errorf("missing %s", name)
		}
	}
	for name := range files {
		if strings.HasSuffix(name, ".so") {
			t.Errorf("unexpected plugin in cross-package: %s", name)
		}
	}
}

func TestFcitxVersionRange(t *testing.T) {
	for _, tc := range []struct{ version, min, max string }{
		{"5.0.21", "5.0", "5.1"}, {"5.1.14", "5.1", "5.2"},
	} {
		min, max, err := fcitxVersionRange(tc.version)
		if err != nil || min != tc.min || max != tc.max {
			t.Errorf("%s interval = [%s, %s), %v", tc.version, min, max, err)
		}
	}
}
