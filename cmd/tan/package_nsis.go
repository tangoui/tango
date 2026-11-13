package main

// The NSIS installer script. tan renders it with text/template from the
// variables in nsisVars; a project replaces the built-in template with
// "installer.windows.template" and adds to it with
// "installer.windows.include". Written against the NSIS 3 manual and the
// Modern UI 2 documentation (MUI2.nsh). See docs/packaging.md.

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// nsisLanguage is one installer language: the tan.json tag it came from
// and the NSIS language name (Contrib/Language files/<Name>.nlf).
type nsisLanguage struct {
	Tag  string // e.g. "zh-Hans"
	Name string // e.g. "SimpChinese"
}

// nsisLanguageNames maps lower-case language tags, with and without a
// region or script, to NSIS language names.
var nsisLanguageNames = map[string]string{
	"en": "English", "en-us": "English", "en-gb": "English",
	"zh": "SimpChinese", "zh-hans": "SimpChinese", "zh-cn": "SimpChinese", "zh-sg": "SimpChinese",
	"zh-hant": "TradChinese", "zh-tw": "TradChinese", "zh-hk": "TradChinese", "zh-mo": "TradChinese",
	"ja": "Japanese", "ko": "Korean",
	"fr": "French", "de": "German", "es": "Spanish", "it": "Italian",
	"pt": "Portuguese", "pt-br": "PortugueseBR", "pt-pt": "Portuguese",
	"ru": "Russian", "uk": "Ukrainian", "pl": "Polish", "cs": "Czech", "sk": "Slovak",
	"nl": "Dutch", "sv": "Swedish", "da": "Danish", "fi": "Finnish", "nb": "Norwegian", "no": "Norwegian", "nn": "NorwegianNynorsk",
	"tr": "Turkish", "el": "Greek", "hu": "Hungarian", "ro": "Romanian", "bg": "Bulgarian", "hr": "Croatian", "sr": "Serbian", "sl": "Slovenian",
	"ar": "Arabic", "he": "Hebrew", "fa": "Farsi", "hi": "Hindi", "th": "Thai", "vi": "Vietnamese", "id": "Indonesian", "ms": "Malay",
	"ca": "Catalan", "et": "Estonian", "lv": "Latvian", "lt": "Lithuanian",
}

// nsisLanguageName maps a language tag to its NSIS name, or "".
func nsisLanguageName(tag string) string {
	t := strings.ToLower(strings.ReplaceAll(tag, "_", "-"))
	for t != "" {
		if n := nsisLanguageNames[t]; n != "" {
			return n
		}
		i := strings.LastIndexByte(t, '-')
		if i < 0 {
			break
		}
		t = t[:i]
	}
	return ""
}

// installerLanguages are the installer's languages: the app's
// localizations (AppleLocalizations: "localizations" or Tango's built-in
// four) in order, each NSIS language once, and English last when the list
// lacks it. The first is the fallback when Windows' language is not among
// them. unknown lists the tags NSIS has no language for.
func installerLanguages(cfg *Config) (langs []nsisLanguage, unknown []string) {
	seen := map[string]bool{}
	for _, tag := range cfg.AppleLocalizations() {
		n := nsisLanguageName(tag)
		switch {
		case n == "":
			unknown = append(unknown, tag)
		case !seen[n]:
			seen[n] = true
			langs = append(langs, nsisLanguage{Tag: tag, Name: n})
		}
	}
	if !seen["English"] {
		langs = append(langs, nsisLanguage{Tag: "en", Name: "English"})
	}
	return langs, unknown
}

// nsisStrings are the script's own texts (LangString), by NSIS language;
// languages missing here use English. The Modern UI translates its own.
var nsisStrings = map[string]map[string]string{
	"TanDesktopShortcut": {
		"English":     "Create a desktop shortcut",
		"SimpChinese": "创建桌面快捷方式",
		"TradChinese": "建立桌面捷徑",
		"Japanese":    "デスクトップにショートカットを作成する",
	},
}

// nsisVars are the template variables, documented in docs/packaging.md.
// Paths are absolute; empty strings and false mean the feature is off.
type nsisVars struct {
	Name, ID, Version, Copyright, Arch string
	VersionQuad                        string // VIProductVersion: 1.2.3.0
	Exe                                string // installed file name, e.g. Hello.exe
	ExeSource                          string // the built executable
	OutFile                            string // the setup.exe to write
	Scope                              string // user | machine
	RegRoot                            string // HKCU | HKLM
	ExecutionLevel                     string // user | admin
	InstallDir                         string // NSIS expression, e.g. $LOCALAPPDATA\Programs\Hello
	UninstallKey                       string // under RegRoot
	ShortcutName                       string // Start menu and desktop shortcut, without .lnk
	Languages                          []nsisLanguage
	LanguageDialog                     bool
	Icon                               string // .ico for the installer and uninstaller
	SidebarImage, HeaderImage          string // .bmp
	License                            string
	Include                            string
	Welcome, RunAfterInstall           bool
	// RunUnelevated: the finish page's Run starts the app through
	// Explorer (TanRunApp) instead of directly, so a machine-wide
	// installer, which runs elevated, does not hand the app its token.
	RunUnelevated   bool
	DesktopShortcut string // ask | always | never
	// RegisterAssociations and UnregisterAssociations are NSIS lines
	// registering the URL schemes and file types (empty without any).
	RegisterAssociations, UnregisterAssociations string
	// LangStrings are the LangString lines of the script's own texts.
	LangStrings string
}

// nsisAssets are the files tan prepared for the script.
type nsisAssets struct {
	icon, sidebar, header string
}

// nsisQuote quotes a literal NSIS string: $ and " are escaped.
func nsisQuote(s string) string {
	s = strings.ReplaceAll(s, "$", "$$")
	return nsisQuoteVars(s)
}

// nsisLiteral escapes s for use inside a quoted NSIS string that the
// template writes itself ("$SMPROGRAMS\{{lit .ShortcutName}}.lnk"): $ is
// written $$ (the NSIS manual's constant for a literal $) and " as $\".
func nsisLiteral(s string) string {
	q := nsisQuote(s)
	return q[1 : len(q)-1]
}

// nsisQuoteVars quotes an NSIS string that may use variables ($INSTDIR).
func nsisQuoteVars(s string) string {
	r := strings.NewReplacer(`"`, `$\"`, "\r", "", "\n", "$\\n")
	return `"` + r.Replace(s) + `"`
}

// shortcutName is a file name made from the app name.
func shortcutName(name string) string {
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) || r < 32 {
			return -1
		}
		return r
	}, name)
	return strings.TrimRight(strings.TrimSpace(name), ".")
}

// newNSISVars computes the template variables.
func newNSISVars(cfg *Config, a artifact, outFile string, as nsisAssets) nsisVars {
	w := cfg.Installer.Windows
	v := nsisVars{
		Name: cfg.Name, ID: cfg.ID, Version: cfg.Version, Copyright: cfg.Copyright, Arch: a.target.Arch,
		VersionQuad:     winVersionString(winVersion(cfg.Version)),
		Exe:             cfg.Exe() + ".exe",
		ExeSource:       a.exe,
		OutFile:         outFile,
		Scope:           "user",
		RegRoot:         "HKCU",
		ExecutionLevel:  "user",
		UninstallKey:    `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + cfg.ID,
		ShortcutName:    cmp.Or(shortcutName(cfg.Name), cfg.Exe()),
		LanguageDialog:  w.LanguageDialog,
		Icon:            as.icon,
		SidebarImage:    as.sidebar,
		HeaderImage:     as.header,
		Welcome:         w.Welcome == nil || *w.Welcome,
		RunAfterInstall: w.RunAfterInstall == nil || *w.RunAfterInstall,
		DesktopShortcut: cmp.Or(w.DesktopShortcut, "ask"),
	}
	dir := nsisLiteral(v.ShortcutName)
	v.InstallDir = `$LOCALAPPDATA\Programs\` + dir
	if w.Scope == "machine" {
		v.Scope, v.RegRoot, v.ExecutionLevel = "machine", "HKLM", "admin"
		// The installer runs elevated, possibly as another account (an
		// administrator who typed their password for a standard user):
		// Run on the finish page would start the app with that token. So
		// it is off unless asked for, and then goes through Explorer.
		v.RunAfterInstall = w.RunAfterInstall != nil && *w.RunAfterInstall
		v.RunUnelevated = v.RunAfterInstall
		pf := "$PROGRAMFILES64"
		if a.target.Arch == "386" {
			pf = "$PROGRAMFILES"
		}
		v.InstallDir = pf + `\` + dir
	}
	if w.License != "" {
		v.License = cfg.Path(w.License)
	}
	if w.Include != "" {
		v.Include = cfg.Path(w.Include)
	}
	v.Languages, _ = installerLanguages(cfg)
	v.RegisterAssociations, v.UnregisterAssociations = nsisAssociations(cfg, v.RegRoot)
	var ls strings.Builder
	for _, key := range sortedKeys(nsisStrings) {
		for _, l := range v.Languages {
			text := cmp.Or(nsisStrings[key][l.Name], nsisStrings[key]["English"])
			fmt.Fprintf(&ls, "LangString %s ${LANG_%s} %s\n", key, strings.ToUpper(l.Name), nsisQuote(text))
		}
	}
	v.LangStrings = ls.String()
	return v
}

// nsisAssociations renders the registry lines for the URL schemes and
// file associations under <root>\Software\Classes, and their removal.
func nsisAssociations(cfg *Config, root string) (install, uninstall string) {
	exe := cfg.Exe() + ".exe"
	q, qv := nsisQuote, nsisQuoteVars
	// The shell command: the quoted executable, "--" so the app does not
	// take the argument for an option (package tango's ParseArgs stops
	// reading options at "--"), and the quoted argument.
	command := `'"$INSTDIR\` + exe + `" -- "%1"'`
	var in, un strings.Builder
	reg := func(key, name, value string) {
		fmt.Fprintf(&in, "  WriteRegStr %s %s %s %s\n", root, q(key), q(name), value)
	}
	icon := qv(`$INSTDIR\` + exe + `,0`)
	for _, sc := range cfg.URLSchemes {
		key := `Software\Classes\` + sc
		reg(key, "", q("URL:"+cfg.Name))
		reg(key, "URL Protocol", `""`)
		reg(key+`\DefaultIcon`, "", icon)
		reg(key+`\shell\open\command`, "", command)
		fmt.Fprintf(&un, "  DeleteRegKey %s %s\n", root, q(key))
	}
	for _, f := range cfg.FileAssociations {
		progID := cfg.ID + "." + strings.ToLower(f.Ext)
		key := `Software\Classes\` + progID
		extKey := `Software\Classes\.` + strings.ToLower(f.Ext)
		reg(key, "", q(f.Name))
		reg(key+`\DefaultIcon`, "", icon)
		reg(key+`\shell\open\command`, "", command)
		// Offer the app in Open with; take the extension's default only
		// when nothing owns it yet.
		reg(extKey+`\OpenWithProgids`, progID, `""`)
		fmt.Fprintf(&in, "  ReadRegStr $0 %s %s \"\"\n  StrCmp $0 \"\" 0 +2\n", root, q(extKey))
		reg(extKey, "", q(progID))
		if f.MIME != "" {
			reg(extKey, "Content Type", q(f.MIME))
		}
		fmt.Fprintf(&un, "  DeleteRegKey %s %s\n  DeleteRegValue %s %s %s\n", root, q(key), root, q(extKey+`\OpenWithProgids`), q(progID))
		fmt.Fprintf(&un, "  ReadRegStr $0 %s %s \"\"\n  StrCmp $0 %s 0 +2\n  DeleteRegValue %s %s \"\"\n", root, q(extKey), q(progID), root, q(extKey))
	}
	if len(cfg.URLSchemes)+len(cfg.FileAssociations) > 0 {
		// SHCNE_ASSOCCHANGED: let Explorer pick up the new associations.
		notify := "  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'\n"
		in.WriteString(notify)
		un.WriteString(notify)
	}
	return in.String(), un.String()
}

var nsisFuncs = template.FuncMap{
	"q":     nsisQuote,     // literal string
	"qv":    nsisQuoteVars, // string with NSIS variables
	"lit":   nsisLiteral,   // literal text inside a quoted string
	"upper": strings.ToUpper,
}

// nsisDefaultTemplate is the built-in installer: optional welcome and
// licence pages, the directory and progress pages, a finish page offering
// to run the app and to create a desktop shortcut; a Start menu shortcut,
// an uninstaller, the Apps & features entry and the associations.
const nsisDefaultTemplate = `; Generated by tan package from tan.json. Build with: makensis installer.nsi
Unicode true
ManifestDPIAware true
!include "MUI2.nsh"
{{- if .Include}}
!include {{q .Include}}
{{- end}}

Name {{q .Name}}
OutFile {{q .OutFile}}
InstallDir {{qv .InstallDir}}
InstallDirRegKey {{.RegRoot}} {{q .UninstallKey}} "InstallLocation"
RequestExecutionLevel {{.ExecutionLevel}}
SetCompressor /SOLID lzma

!define MUI_ABORTWARNING
{{- if .Icon}}
!define MUI_ICON {{q .Icon}}
!define MUI_UNICON {{q .Icon}}
{{- end}}
{{- if .SidebarImage}}
!define MUI_WELCOMEFINISHPAGE_BITMAP {{q .SidebarImage}}
!define MUI_UNWELCOMEFINISHPAGE_BITMAP {{q .SidebarImage}}
{{- end}}
{{- if .HeaderImage}}
!define MUI_HEADERIMAGE
!define MUI_HEADERIMAGE_BITMAP {{q .HeaderImage}}
!define MUI_HEADERIMAGE_UNBITMAP {{q .HeaderImage}}
{{- end}}
{{- if .LanguageDialog}}
!define MUI_LANGDLL_REGISTRY_ROOT {{.RegRoot}}
!define MUI_LANGDLL_REGISTRY_KEY {{q .UninstallKey}}
!define MUI_LANGDLL_REGISTRY_VALUENAME "InstallerLanguage"
{{- end}}

{{if .Welcome}}!insertmacro MUI_PAGE_WELCOME
{{end -}}
{{if .License}}!insertmacro MUI_PAGE_LICENSE {{q .License}}
{{end -}}
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
{{- if .RunUnelevated}}
!define MUI_FINISHPAGE_RUN
!define MUI_FINISHPAGE_RUN_FUNCTION TanRunApp
{{- else if .RunAfterInstall}}
!define MUI_FINISHPAGE_RUN "$INSTDIR\{{.Exe}}"
{{- end}}
{{- if eq .DesktopShortcut "ask"}}
!define MUI_FINISHPAGE_SHOWREADME ""
!define MUI_FINISHPAGE_SHOWREADME_TEXT "$(TanDesktopShortcut)"
!define MUI_FINISHPAGE_SHOWREADME_FUNCTION TanDesktopShortcut
{{- end}}
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

{{range .Languages}}!insertmacro MUI_LANGUAGE "{{.Name}}"
{{end -}}
{{if .LanguageDialog}}!insertmacro MUI_RESERVEFILE_LANGDLL
{{end -}}
{{.LangStrings}}
VIProductVersion "{{.VersionQuad}}"
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductName" {{q .Name}}
VIAddVersionKey /LANG=${LANG_ENGLISH} "ProductVersion" {{q .Version}}
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileVersion" {{q .Version}}
VIAddVersionKey /LANG=${LANG_ENGLISH} "FileDescription" {{q (print .Name " Setup")}}
VIAddVersionKey /LANG=${LANG_ENGLISH} "LegalCopyright" {{q .Copyright}}

Function .onInit
{{- if eq .Scope "machine"}}
  SetShellVarContext all
{{- end}}
{{- if .LanguageDialog}}
  IfSilent tan_lang_done
  !insertmacro MUI_LANGDLL_DISPLAY
  tan_lang_done:
{{- end}}
  !ifmacrodef customInit
    !insertmacro customInit
  !endif
FunctionEnd

Section "Install"
  SetOutPath "$INSTDIR"
  File {{q .ExeSource}}
  WriteUninstaller "$INSTDIR\uninstall.exe"
  CreateShortcut "$SMPROGRAMS\{{lit .ShortcutName}}.lnk" "$INSTDIR\{{.Exe}}"
{{- if eq .DesktopShortcut "always"}}
  CreateShortcut "$DESKTOP\{{lit .ShortcutName}}.lnk" "$INSTDIR\{{.Exe}}"
{{- end}}
  WriteRegStr {{.RegRoot}} "{{.UninstallKey}}" "DisplayName" {{q .Name}}
  WriteRegStr {{.RegRoot}} "{{.UninstallKey}}" "DisplayVersion" {{q .Version}}
  WriteRegStr {{.RegRoot}} "{{.UninstallKey}}" "DisplayIcon" "$INSTDIR\{{.Exe}}"
  WriteRegStr {{.RegRoot}} "{{.UninstallKey}}" "InstallLocation" "$INSTDIR"
  WriteRegStr {{.RegRoot}} "{{.UninstallKey}}" "UninstallString" '"$INSTDIR\uninstall.exe"'
  WriteRegStr {{.RegRoot}} "{{.UninstallKey}}" "QuietUninstallString" '"$INSTDIR\uninstall.exe" /S'
  WriteRegDWORD {{.RegRoot}} "{{.UninstallKey}}" "NoModify" 1
  WriteRegDWORD {{.RegRoot}} "{{.UninstallKey}}" "NoRepair" 1
{{.RegisterAssociations}}  !ifmacrodef customInstall
    !insertmacro customInstall
  !endif
SectionEnd
{{- if eq .DesktopShortcut "ask"}}

Function TanDesktopShortcut
  CreateShortcut "$DESKTOP\{{lit .ShortcutName}}.lnk" "$INSTDIR\{{.Exe}}"
FunctionEnd
{{- end}}
{{- if .RunUnelevated}}

; Run from the finish page of an elevated installer: Explorer, already
; running as the signed-in user, starts the app instead of this process.
Function TanRunApp
  Exec '"$WINDIR\explorer.exe" "$INSTDIR\{{.Exe}}"'
FunctionEnd
{{- end}}

Function un.onInit
{{- if eq .Scope "machine"}}
  SetShellVarContext all
{{- end}}
{{- if .LanguageDialog}}
  !insertmacro MUI_UNGETLANGUAGE
{{- end}}
FunctionEnd

Section "Uninstall"
  !ifmacrodef customUnInstall
    !insertmacro customUnInstall
  !endif
{{.UnregisterAssociations}}  Delete "$INSTDIR\{{.Exe}}"
  Delete "$INSTDIR\uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\{{lit .ShortcutName}}.lnk"
  Delete "$DESKTOP\{{lit .ShortcutName}}.lnk"
  DeleteRegKey {{.RegRoot}} "{{.UninstallKey}}"
SectionEnd
`

var nsisDefault = template.Must(template.New("installer.nsi").Funcs(nsisFuncs).Parse(nsisDefaultTemplate))

// utf8BOM starts the script so makensis reads it as UTF-8 whatever the
// host's code page.
const utf8BOM = "\uFEFF"

// nsisScript renders the built-in script without prepared assets (no
// icon or converted images).
func nsisScript(cfg *Config, a artifact, outFile string) string {
	var b strings.Builder
	if err := nsisDefault.Execute(&b, newNSISVars(cfg, a, outFile, nsisAssets{})); err != nil {
		panic(err) // the built-in template is tested
	}
	return utf8BOM + b.String()
}

// renderNSIS renders the script for a package: the project's template
// when it has one, else the built-in one.
func renderNSIS(cfg *Config, vars nsisVars) (string, error) {
	tmpl := nsisDefault
	if p := cfg.Installer.Windows.Template; p != "" {
		src, err := os.ReadFile(cfg.Path(p))
		if err != nil {
			return "", fmt.Errorf(`"installer.windows.template": %w`, err)
		}
		tmpl, err = template.New(filepath.Base(p)).Funcs(nsisFuncs).Option("missingkey=error").Parse(string(src))
		if err != nil {
			return "", fmt.Errorf(`"installer.windows.template": %w`, err)
		}
	}
	var b strings.Builder
	if err := tmpl.Execute(&b, vars); err != nil {
		return "", fmt.Errorf(`"installer.windows.template": %w`, err)
	}
	return utf8BOM + strings.TrimPrefix(b.String(), utf8BOM), nil
}

// writeNSISScript prepares the installer's files in a.dir (icon,
// converted images), checks the project files it names, and writes
// installer.nsi.
func writeNSISScript(cfg *Config, a artifact, outFile string) (string, error) {
	w := cfg.Installer.Windows
	if langs, unknown := installerLanguages(cfg); len(unknown) > 0 {
		warnf("The NSIS installer has no language for %s; it uses %s", strings.Join(unknown, ", "), langs[0].Name)
	}
	if w.Scope == "machine" && w.RunAfterInstall != nil && *w.RunAfterInstall {
		warnf(`"installer.windows.runAfterInstall" with scope "machine": the finish page starts the app through Explorer (explorer.exe <app>) so it runs in the signed-in user's shell rather than with the installer's administrator token; this relies on Explorer's behaviour, so test it on your target Windows versions`)
	}
	for name, p := range map[string]string{"license": w.License, "include": w.Include} {
		if p == "" {
			continue
		}
		if _, err := os.Stat(cfg.Path(p)); err != nil {
			return "", fmt.Errorf(`"installer.windows.%s": %w`, name, err)
		}
	}
	var as nsisAssets
	frames, err := windowsIconFrames(cfg)
	if err != nil {
		return "", err
	}
	if len(frames) > 0 {
		as.icon = filepath.Join(a.dir, "installer.ico")
		if err := os.WriteFile(as.icon, icoFile(frames), 0o644); err != nil {
			return "", err
		}
	}
	if as.sidebar, err = nsisBitmap(cfg, "sidebarImage", w.SidebarImage, filepath.Join(a.dir, "installer-sidebar.bmp")); err != nil {
		return "", err
	}
	if as.header, err = nsisBitmap(cfg, "headerImage", w.HeaderImage, filepath.Join(a.dir, "installer-header.bmp")); err != nil {
		return "", err
	}
	script, err := renderNSIS(cfg, newNSISVars(cfg, a, outFile, as))
	if err != nil {
		return "", err
	}
	nsi := filepath.Join(a.dir, "installer.nsi")
	return nsi, os.WriteFile(nsi, []byte(script), 0o644)
}

// nsisBitmap returns the BMP for an image setting: a .bmp as is, a PNG
// (or JPEG) converted to a 24-bit BMP at out, flattened onto white.
func nsisBitmap(cfg *Config, key, rel, out string) (string, error) {
	if rel == "" {
		return "", nil
	}
	src := cfg.Path(rel)
	b, err := os.ReadFile(src)
	if err != nil {
		return "", fmt.Errorf(`"installer.windows.%s": %w`, key, err)
	}
	if bytes.HasPrefix(b, []byte("BM")) {
		return src, nil
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return "", fmt.Errorf(`"installer.windows.%s" %s: %w`, key, rel, err)
	}
	return out, os.WriteFile(out, encodeBMP(img), 0o644)
}

// encodeBMP writes img as an uncompressed 24-bit, bottom-up BMP
// (BITMAPFILEHEADER and BITMAPINFOHEADER), composited onto white.
func encodeBMP(img image.Image) []byte {
	r := img.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(flat, flat.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), img, r.Min, draw.Over)
	w, h := r.Dx(), r.Dy()
	stride := (w*3 + 3) &^ 3
	size := 54 + stride*h
	le := binary.LittleEndian
	b := make([]byte, 0, size)
	b = append(b, 'B', 'M')
	b = le.AppendUint32(b, uint32(size))
	b = le.AppendUint32(b, 0)
	b = le.AppendUint32(b, 54)
	b = le.AppendUint32(b, 40)
	b = le.AppendUint32(b, uint32(w))
	b = le.AppendUint32(b, uint32(h))
	b = le.AppendUint16(b, 1)  // planes
	b = le.AppendUint16(b, 24) // bits per pixel
	b = le.AppendUint32(b, 0)  // BI_RGB
	b = le.AppendUint32(b, uint32(stride*h))
	b = le.AppendUint32(b, 2835) // 72 dpi
	b = le.AppendUint32(b, 2835)
	b = le.AppendUint32(b, 0)
	b = le.AppendUint32(b, 0)
	for y := h - 1; y >= 0; y-- {
		row := flat.Pix[y*flat.Stride:]
		for x := range w {
			p := row[x*4:]
			b = append(b, p[2], p[1], p[0])
		}
		for range stride - w*3 {
			b = append(b, 0)
		}
	}
	return b
}

// makensisHint says how to install NSIS on this host.
func makensisHint(goos string) string {
	switch goos {
	case "darwin":
		return "brew install makensis"
	case "linux":
		return "sudo apt install nsis (Debian, Ubuntu) or sudo dnf install mingw32-nsis (Fedora)"
	}
	return "install NSIS from https://nsis.sourceforge.io"
}
