package main

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var inputModeIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

func (im *InputMethod) validateModes(field string) []error {
	var errs []error
	seen := make(map[string]bool)
	for i, mode := range im.Modes {
		path := fmt.Sprintf("%s.modes[%d]", field, i)
		switch id := strings.ToLower(mode.ID); {
		case !inputModeIDPattern.MatchString(id):
			errs = append(errs, fmt.Errorf("%s.id %q must contain only letters, digits and hyphens", path, mode.ID))
		case seen[id]:
			errs = append(errs, fmt.Errorf("%s.id %q is a duplicate", path, mode.ID))
		default:
			seen[id] = true
		}
		if strings.TrimSpace(mode.Label) == "" {
			errs = append(errs, fmt.Errorf("%s.label is required", path))
		}
		if strings.TrimSpace(mode.Short) == "" {
			errs = append(errs, fmt.Errorf("%s.short is required", path))
		}
		if configPathAbs(mode.Icon) {
			errs = append(errs, fmt.Errorf("%s.icon path %q must be relative to the project", path, mode.Icon))
		}
	}
	return errs
}

func (mode InputMode) iconFile() string {
	return "mode-" + mode.ID + filepath.Ext(mode.Icon)
}

func writeInputMethodResources(cfg *Config, res string) error {
	im := cfg.inputMethod()
	if im == nil {
		return nil
	}
	if im.Icon != "" {
		if err := copyFile(cfg.Path(im.Icon), filepath.Join(res, "menuicon.png"), 0o644); err != nil {
			return fmt.Errorf("inputMethod.icon: %w", err)
		}
	}
	if len(im.Modes) == 0 {
		return nil
	}
	var names bytes.Buffer
	names.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
`)
	for _, mode := range im.Modes {
		if mode.Icon != "" {
			if err := copyFile(cfg.Path(mode.Icon), filepath.Join(res, mode.iconFile()), 0o644); err != nil {
				return fmt.Errorf("inputMethod mode %q icon %q: %w", mode.ID, mode.Icon, err)
			}
		}
		plistString(&names, "\t", cfg.ID+"."+mode.ID, mode.Label)
	}
	names.WriteString("</dict>\n</plist>\n")
	dir := filepath.Join(res, "en.lproj")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "InfoPlist.strings"), names.Bytes(), 0o644)
}

func plistString(b *bytes.Buffer, indent, key, value string) {
	fmt.Fprintf(b, "%s<key>%s</key>\n%s<string>%s</string>\n", indent, html.EscapeString(key), indent, html.EscapeString(value))
}

func writeInputModes(b *bytes.Buffer, bundleID string, im *InputMethod, icon string) {
	menuIcon := ""
	if im.Icon != "" {
		menuIcon = "menuicon.png"
	} else if icon != "" {
		menuIcon = icon + ".icns"
	}
	language := im.Language
	if language == "" {
		language = "zh-Hans"
	}

	plistString(b, "\t", "TISInputSourceID", bundleID)
	b.WriteString("\t<key>ComponentInputModeDict</key>\n\t<dict>\n")
	b.WriteString("\t\t<key>tsInputModeListKey</key>\n\t\t<dict>\n")
	ids := make([]string, 0, len(im.Modes))
	for i, mode := range im.Modes {
		id := bundleID + "." + mode.ID
		ids = append(ids, id)
		fmt.Fprintf(b, "\t\t\t<key>%s</key>\n\t\t\t<dict>\n", html.EscapeString(id))
		const indent = "\t\t\t\t"
		plistString(b, indent, "TISInputSourceID", id)
		lang := language
		if mode.ID == "en" {
			lang = "en"
		}
		plistString(b, indent, "TISIntendedLanguage", lang)
		plistString(b, indent, "tsInputModeScriptKey", "smUnicodeScript")
		plistStrings(b, indent, "tsInputModeCharacterRepertoireKey", []string{repertoire(lang)})
		file := menuIcon
		if mode.Icon != "" {
			file = mode.iconFile()
		}
		plistString(b, indent, "tsInputModeMenuIconFileKey", file)
		b.WriteString(indent + "<key>TISIconLabels</key>\n" + indent + "<dict>\n")
		plistString(b, indent+"\t", "Primary", mode.Short)
		b.WriteString(indent + "</dict>\n")
		b.WriteString(indent + "<key>tsInputModeIsVisibleKey</key>\n" + indent + "<true/>\n")
		b.WriteString(indent + "<key>tsInputModeDefaultStateKey</key>\n" + indent + "<true/>\n")
		fmt.Fprintf(b, "%s<key>tsInputModePrimaryInScriptKey</key>\n%s<%t/>\n", indent, indent, i == 0)
		b.WriteString("\t\t\t</dict>\n")
	}
	b.WriteString("\t\t</dict>\n")
	plistStrings(b, "\t\t", "tsVisibleInputModeOrderedArrayKey", ids)
	b.WriteString("\t</dict>\n")
	b.WriteString("\t<key>TangoInputModes</key>\n\t<array>\n")
	for _, mode := range im.Modes {
		b.WriteString("\t\t<dict>\n")
		plistString(b, "\t\t\t", "id", mode.ID)
		plistString(b, "\t\t\t", "label", mode.Label)
		plistString(b, "\t\t\t", "short", mode.Short)
		b.WriteString("\t\t</dict>\n")
	}
	b.WriteString("\t</array>\n")
}
