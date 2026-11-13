package theme

import (
	"strings"
	"testing"
)

func TestCSSVarsAnt(t *testing.T) {
	v := AntDesign().CSSVars()
	want := map[string]string{
		"--tango-color-primary":          "#1677ff",
		"--tango-color-primary-hover":    "#4096ff",
		"--tango-color-primary-active":   "#0958d9",
		"--tango-color-primary-bg":       "#e6f4ff",
		"--tango-color-primary-1":        "#e6f4ff",
		"--tango-color-primary-10":       "#001d66",
		"--tango-color-success":          "#52c41a",
		"--tango-color-error-hover":      "#ff7875",
		"--tango-color-text":             "rgba(0, 0, 0, 0.88)",
		"--tango-color-border":           "#d9d9d9",
		"--tango-color-bg-container":     "#ffffff",
		"--tango-color-bg-layout":        "#f5f5f5",
		"--tango-radius":                 "6px",
		"--tango-radius-lg":              "8px",
		"--tango-font-size":              "14px",
		"--tango-font-size-heading-1":    "38px",
		"--tango-line-height":            "1.5714",
		"--tango-font-weight-strong":     "600",
		"--tango-control-height":         "32px",
		"--tango-control-height-sm":      "24px",
		"--tango-padding":                "16px",
		"--tango-motion-duration-mid":    "0.2s",
		"--tango-shadow-tertiary":        "0 1px 2px 0 rgba(0, 0, 0, 0.03), 0 1px 6px -1px rgba(0, 0, 0, 0.02), 0 2px 4px 0 rgba(0, 0, 0, 0.02)",
		"--tango-color-scheme":           "light",
		"--tango-color-info":             "#1677ff",
		"--tango-color-warning-border":   HexString(AntDesign().Warning.Border),
		"--tango-color-text-on-primary":  "#ffffff",
		"--tango-color-bg-mask":          "rgba(0, 0, 0, 0.45)",
		"--tango-color-link":             "#1677ff",
		"--tango-color-border-secondary": "#f0f0f0",
	}
	for k, w := range want {
		if got, ok := v[k]; !ok || got != w {
			t.Errorf("%s = %q (present %v), want %q", k, got, ok, w)
		}
	}
	for k, s := range v {
		if !strings.HasPrefix(k, CSSPrefix) || s == "" || strings.ContainsAny(s, ";{}") {
			t.Errorf("bad variable %q: %q", k, s)
		}
	}
}

func TestCSSVarsElementDark(t *testing.T) {
	light, dark := ElementPlus().CSSVars(), ElementPlus(Dark()).CSSVars()
	if light["--tango-color-primary"] != "#409eff" || light["--tango-radius"] != "4px" {
		t.Fatalf("element tokens: %s %s", light["--tango-color-primary"], light["--tango-radius"])
	}
	if dark["--tango-color-scheme"] != "dark" || dark["--tango-color-bg-container"] != "#141414" || dark["--tango-color-text"] != "#e5eaf3" {
		t.Fatalf("dark tokens: %v %v %v", dark["--tango-color-scheme"], dark["--tango-color-bg-container"], dark["--tango-color-text"])
	}
	if len(light) != len(dark) {
		t.Fatalf("light and dark export different sets: %d vs %d", len(light), len(dark))
	}
	// A custom primary flows into every derived shade.
	custom := AntDesign(WithPrimary(Hex("#722ed1"))).CSSVars()
	if custom["--tango-color-primary"] != "#722ed1" || custom["--tango-color-primary-1"] == "#e6f4ff" {
		t.Fatal("custom primary not exported")
	}
}

func TestCSSRule(t *testing.T) {
	got := CSSRule(":root", map[string]string{"--tango-b": "2px", "--tango-a": "#fff"})
	want := ":root {\n  --tango-a: #fff;\n  --tango-b: 2px;\n}\n"
	if got != want {
		t.Fatalf("CSSRule:\n%s", got)
	}
	css := AntDesign().CSS()
	if !strings.HasPrefix(css, ":root {\n  --tango-color-bg-container: #ffffff;\n") || !strings.HasSuffix(css, "}\n") {
		t.Fatalf("CSS:\n%.200s", css)
	}
	if css != AntDesign().CSS() {
		t.Fatal("output not stable")
	}
	if n := strings.Count(css, ";\n"); n != len(AntDesign().CSSVars()) {
		t.Fatalf("%d declarations", n)
	}
}

func TestCSSNumbers(t *testing.T) {
	for in, want := range map[float32]string{0: "0", 14: "14px", 1.5: "1.5px", -4: "-4px", 0.25: "0.25px"} {
		if got := cssPx(in); got != want {
			t.Errorf("cssPx(%v) = %q, want %q", in, got, want)
		}
	}
}
