package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type buildFeatures uint8

const (
	featureAll buildFeatures = iota
	featureWebView
	featureCanvas
	featureMixed
)

func (f buildFeatures) String() string {
	switch f {
	case featureWebView:
		return "webview"
	case featureCanvas:
		return "canvas"
	case featureMixed:
		return "webview,canvas"
	default:
		return "all"
	}
}

func parseFeatures(raw string) (buildFeatures, error) {
	if raw == "" {
		return featureAll, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("features cannot be empty")
	}
	if raw == "all" {
		return featureAll, nil
	}
	var hasW, hasC bool
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		switch part {
		case "webview":
			if hasW {
				return 0, fmt.Errorf("duplicate feature %q", part)
			}
			hasW = true
		case "canvas":
			if hasC {
				return 0, fmt.Errorf("duplicate feature %q", part)
			}
			hasC = true
		case "all":
			return 0, errors.New("feature all cannot be combined")
		default:
			return 0, fmt.Errorf("unknown feature %q (want webview, canvas, or all)", part)
		}
	}
	if hasW && hasC {
		return featureMixed, nil
	}
	if hasW {
		return featureWebView, nil
	}
	if hasC {
		return featureCanvas, nil
	}
	return 0, errors.New("features cannot be empty")
}
func configFeatures(values []string) (buildFeatures, error) {
	if values == nil {
		return featureAll, nil
	}
	if len(values) == 0 {
		return 0, errors.New("features must not be empty")
	}
	for _, value := range values {
		if value != "webview" && value != "canvas" {
			return 0, fmt.Errorf("invalid config feature %q (want webview or canvas)", value)
		}
	}
	return parseFeatures(strings.Join(values, ","))
}

func resolveFeatures(config []string, override string, hasOverride bool) (buildFeatures, error) {
	if hasOverride {
		if strings.TrimSpace(override) == "" {
			return 0, errors.New("-features cannot be empty")
		}
		return parseFeatures(override)
	}
	return configFeatures(config)
}

func featureTags(f buildFeatures, t target) ([]string, error) {
	if t.OS == "ios" || t.OS == "ios-simulator" || t.OS == "android" {
		return nil, nil // mobile build paths do not use desktop profiles
	}
	if f != featureAll && t.OS != "darwin" {
		return nil, fmt.Errorf("features %s is not implemented on %s (only all is supported)", f, t.OS)
	}
	for _, tag := range goflagsTags(os.Getenv("GOFLAGS")) {
		if strings.HasPrefix(tag, "tango_backend_") {
			return nil, fmt.Errorf("GOFLAGS contains conflicting Tango backend tag %q; use -features instead", tag)
		}
	}
	switch f {
	case featureWebView:
		return []string{"tango_backend_webview"}, nil
	case featureCanvas:
		return []string{"tango_backend_canvas"}, nil
	case featureMixed:
		return []string{"tango_backend_webview", "tango_backend_canvas"}, nil
	}
	return nil, nil
}

// oneStringFlag rejects repeated -features flags instead of silently taking
// the last value.
type oneStringFlag struct {
	value string
	set   bool
}

func (f *oneStringFlag) String() string { return f.value }
func (f *oneStringFlag) Set(v string) error {
	if f.set {
		return errors.New("-features may be specified only once")
	}
	f.set = true
	f.value = v
	return nil
}
