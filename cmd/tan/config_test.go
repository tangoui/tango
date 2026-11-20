package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseConfig(t *testing.T) {
	good := `{
  "name": "My App",
  "id": "com.example.myapp",
  "version": "1.2.3",
  "icon": "icon.png",
  "frontend": {"dir": "frontend", "dist": "dist", "dev": "npm run dev", "build": "npm run build"},
  "macos": {"minVersion": "12.0"},
  "urlSchemes": ["myapp"],
  "fileAssociations": [{"ext": "md", "name": "Markdown", "role": "viewer", "mime": "text/markdown"}]
}`
	c, err := ParseConfig([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.URLSchemes) != 1 || !c.FileAssociations[0].Viewer() || c.FileAssociations[0].MIMEType(c) != "text/markdown" {
		t.Fatalf("associations %+v %+v", c.URLSchemes, c.FileAssociations)
	}
	if c.Exe() != "MyApp" || c.Frontend.Dev != "npm run dev" || c.MacOS.MinVersion != "12.0" {
		t.Fatalf("parsed %+v", c)
	}

	bad := []struct{ json, want string }{
		{`{"name":"A","id":"x","version":"1.0.0","frontend":{"dir":"f","dist":"."}}`, `"id"`},
		{`{"name":"A","id":"a.b","version":"one","frontend":{"dir":"f","dist":"."}}`, `"version"`},
		{`{"name":"","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `"name"`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f"}}`, `frontend.dist`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":".","icn":"x"}}`, `unknown field "icn"`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"/abs","dist":"."}}`, `relative`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"urlSchemes":["https"]}`, `"urlSchemes"`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"urlSchemes":["My_App"]}`, `"urlSchemes"`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"fileAssociations":[{"ext":".md","name":"M"}]}`, `no dot`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"fileAssociations":[{"ext":"md"}]}`, `"name"`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"fileAssociations":[{"ext":"md","name":"M","role":"owner"}]}`, `role`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"fileAssociations":[{"ext":"md","name":"M"},{"ext":"MD","name":"M"}]}`, `twice`},
		{`{"name":"A\nX-Evil=1","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `control characters`},
		{`{"name":"A\rB","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `control characters`},
		{`{"name":"A\u0000B","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `control characters`},
		{`{"name":"../../evil","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `".."`},
		{`{"name":"a/b","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `'/'`},
		{`{"name":"a\\b","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `'/'`},
		{`{"name":"A..B","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}}`, `".."`},
		{`{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"android":{"keystore":"k.jks","keyAlias":"upload&calc"}}`, `"android.keyAlias"`},
	}
	for _, b := range bad {
		_, err := ParseConfig([]byte(b.json))
		if err == nil || !strings.Contains(err.Error(), b.want) {
			t.Errorf("%s: err = %v, want it to mention %s", b.json, err, b.want)
		}
	}
}

func TestParseParallelDevBuild(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  bool
	}{
		{"omitted", ``, false},
		{"false", `,"parallelDevBuild":false`, false},
		{"true", `,"parallelDevBuild":true`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(`{"name":"Hello","id":"com.example.hello","version":"1.0.0","frontend":{"dir":"frontend","dist":"."` + tc.input + `}}`))
			if err != nil {
				t.Fatal(err)
			}
			// Compare the serialized opt-in as well as accepting all three inputs.
			encoded, err := json.Marshal(cfg.Frontend)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), `"parallelDevBuild":true`) != tc.want {
				t.Fatalf("frontend = %s, want parallelDevBuild enabled: %t", encoded, tc.want)
			}
		})
	}
}

func TestConfigCGO(t *testing.T) {
	base := `{"name":"A","id":"a.b","version":"1.0.0","frontend":{"dir":"f","dist":"."}`
	for _, c := range []struct{ json, want string }{
		{base + `}`, "CGO_ENABLED=0"},
		{base + `,"cgo":false}`, "CGO_ENABLED=0"},
		{base + `,"cgo":true}`, "CGO_ENABLED=1"},
	} {
		cfg, err := ParseConfig([]byte(c.json))
		if err != nil {
			t.Fatalf("%s: %v", c.json, err)
		}
		if got := cfg.cgoEnv(); got != c.want {
			t.Errorf("%s: cgoEnv = %s, want %s", c.json, got, c.want)
		}
	}
}

func TestConfigRoundTrip(t *testing.T) {
	c := &Config{Name: "Hello", ID: "com.example.hello", Version: "0.1.0", Frontend: Frontend{Dir: "frontend", Dist: "."}}
	out := string(c.Marshal())
	if strings.Contains(out, "macos") || strings.Contains(out, "devURL") {
		t.Errorf("empty optional fields written:\n%s", out)
	}
	if _, err := ParseConfig([]byte(out)); err != nil {
		t.Fatal(err)
	}
}

func TestSlugAndExe(t *testing.T) {
	for in, want := range map[string]string{"My App": "my-app", "  Foo__Bar 2 ": "foo-bar-2", "日本": ""} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := (&Config{Name: "Café Tool-2"}).Exe(); got != "CafTool-2" {
		t.Errorf("Exe = %q", got)
	}
}

func TestParseConfigWindows(t *testing.T) {
	const base = `{"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."},"windows":%s}`
	c, err := ParseConfig([]byte(strings.Replace(base, "%s", `{"certificate":"win/cert.pfx","timestampURL":"http://ts.example","publisher":"CN=A, O=A"}`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if c.Windows.Certificate != "win/cert.pfx" || c.Windows.TimestampURL != "http://ts.example" || c.Windows.Publisher != "CN=A, O=A" {
		t.Errorf("windows %+v", c.Windows)
	}
	for w, want := range map[string]string{
		`{"certificate":"/abs/cert.pfx"}`: "relative",
		`{"certificate":"cert.cer"}`:      ".pfx",
		`{"timestampURL":"ts.example"}`:   "timestampURL",
		`{"publisher":"Example Ltd"}`:     "CN=",
		`{"password":"x"}`:                `unknown field "password"`,
	} {
		_, err := ParseConfig([]byte(strings.Replace(base, "%s", w, 1)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want it to mention %s", w, err, want)
		}
	}
	if out := string((&Config{Name: "A", ID: "a.b", Version: "1"}).Marshal()); strings.Contains(out, "windows") {
		t.Errorf("empty windows section written:\n%s", out)
	}
}

// TestConfigPathAbs: tan.json paths are judged absolute the same way on
// every OS.
func TestConfigPathAbs(t *testing.T) {
	for p, want := range map[string]bool{
		"":           false,
		"icon.png":   false,
		"a/b.png":    false,
		`a\b.png`:    false,
		"./x":        false,
		"../x":       false,
		"/abs":       true,
		`\abs`:       true,
		`\server\s`:  true,
		`C:\abs`:     true,
		"c:/abs":     true,
		"C:rel":      true,
		"1:x":        false,
		"weird:name": false,
	} {
		if got := configPathAbs(p); got != want {
			t.Errorf("configPathAbs(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestParseConfigAppFields(t *testing.T) {
	const prefix = `{"name":"App","id":"com.example.app","version":"1.0.0",`
	for _, fields := range []string{`"cloud":{"apiUrl":"https://example.com","nested":{"enabled":true}}`, `"custom":[null,true,1,{"frontend":42}]`, `"flag":false,"count":123456789012345678901,"s":"x","unused":null`} {
		c, err := ParseConfig([]byte(prefix + fields + `}`))
		if err != nil || c.Name != "App" {
			t.Fatalf("%s: %+v, %v", fields, c, err)
		}
	}
	for _, fields := range []string{`"cloud":true,"cgo":"yes"`, `"cloud":{},"features":null`, `"cloud":{},"frontend":{"parallelDevBuld":true}`, `"cloud":1} {`} {
		if _, err := ParseConfig([]byte(prefix + fields + `}`)); err == nil {
			t.Fatalf("accepted invalid config: %s", fields)
		}
	}
}

func TestParseConfigDuplicateFeatures(t *testing.T) {
	const prefix = `{"name":"App","id":"com.example.app","version":"1.0.0","cloud":{},`
	if _, err := ParseConfig([]byte(prefix + `"features":null,"features":["webview"]}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := ParseConfig([]byte(prefix + `"features":["webview"],"features":null}`)); err == nil {
		t.Fatal("accepted final null features")
	}
}
