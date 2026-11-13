package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// ConfigFile is the project file every command but init reads.
const ConfigFile = "tan.json"

// Config is tan.json. Paths are relative to the project directory.
//
//	{
//	  "name": "Hello",
//	  "id": "com.example.hello",
//	  "version": "0.1.0",
//	  "icon": "icon.png",
//	  "icons": {"macos": "icons/macos.png", "android": {"background": "#1677FF"}},
//	  "frontend": {
//	    "dir": "frontend",
//	    "dist": "dist",
//	    "dev": "npm run dev",
//	    "build": "npm run build"
//	  },
//	  "urlSchemes": ["hello"],
//	  "appLinks": ["example.com", "*.example.com"],
//	  "fileAssociations": [{"ext": "hlo", "name": "Hello document"}],
//	  "permissions": ["camera", "location"],
//	  "ios": {
//	    "minVersion": "17.0",
//	    "signIdentity": "Apple Development: Jane Doe (TEAMID)",
//	    "provisioningProfile": "mobile/hello.mobileprovision",
//	    "usageDescriptions": {"camera": "Hello scans QR codes with the camera."}
//	  },
//	  "android": {
//	    "minSdk": 24,
//	    "keystore": "mobile/release.jks",
//	    "keyAlias": "upload"
//	  },
//	  "windows": {
//	    "certificate": "windows/codesign.pfx",
//	    "publisher": "CN=Example Ltd, O=Example Ltd, C=US"
//	  }
//	}
type Config struct {
	Name      string `json:"name"`           // display name; also names the executable (see Exe)
	ID        string `json:"id"`             // reverse-DNS identifier, e.g. com.example.hello
	Version   string `json:"version"`        // x.y.z
	Icon      string `json:"icon,omitempty"` // master icon: square, full-bleed PNG, ideally 1024×1024 (see Icons)
	Icons     Icons  `json:"icons,omitzero"` // per-platform overrides; see icons_config.go
	Copyright string `json:"copyright,omitempty"`
	// Localizations are the languages the app declares to Apple platforms
	// (CFBundleLocalizations); system-provided menu items and dialogs are
	// localized only into these. Default: en, zh-Hans, zh-Hant, ja (the
	// languages Tango's built-in strings ship).
	Localizations []string `json:"localizations,omitempty"`
	Frontend      Frontend `json:"frontend,omitzero"` // omit for a self-drawn UI app
	MacOS         MacOS    `json:"macos,omitzero"`
	Windows       Windows  `json:"windows,omitzero"`
	Linux         Linux    `json:"linux,omitzero"`
	// Installer customises the installers tan package makes: the NSIS
	// setup.exe and the macOS disk image. See docs/packaging.md.
	Installer Installer `json:"installer,omitzero"`

	// URLSchemes are the deep-link schemes the app opens (myapp for
	// myapp://...); the app declares the same list in tango.Options.
	URLSchemes []string `json:"urlSchemes,omitempty"`
	// AppLinks are the web domains ("example.com", "*.example.com") whose
	// https links open the app: iOS Universal Links (associated-domains
	// entitlement) and Android App Links (autoVerify intent filters). The
	// site proves the link with the files tan applinks writes (applinks.go).
	AppLinks []string `json:"appLinks,omitempty"`
	// AppLinkPaths limits AppLinks to these path prefixes ("/open/");
	// default every path.
	AppLinkPaths []string `json:"appLinkPaths,omitempty"`
	// FileAssociations are the document types the app opens.
	FileAssociations []FileAssociation `json:"fileAssociations,omitempty"`

	// Permissions are the mobile capabilities the app uses ("camera",
	// "location", ...; see permissions.go). tan build declares them: iOS
	// usage descriptions in Info.plist, Android <uses-permission> entries.
	Permissions []string `json:"permissions,omitempty"`
	IOS         IOS      `json:"ios,omitzero"`
	Android     Android  `json:"android,omitzero"`

	dir string // project directory (absolute)
}

// Frontend says where the front end lives and how to run it.
type Frontend struct {
	Dir     string `json:"dir"`               // front-end root; commands run here
	Dist    string `json:"dist"`              // build output, relative to Dir ("." when the sources are served as is)
	Install string `json:"install,omitempty"` // run when Dir has package.json but no node_modules; default "npm install"
	Dev     string `json:"dev,omitempty"`     // dev server command; its first printed http URL is used
	DevURL  string `json:"devURL,omitempty"`  // dev server URL, when it cannot be read from the output
	Build   string `json:"build,omitempty"`   // production build command
}

// FileAssociation registers the app for files with an extension.
type FileAssociation struct {
	Ext  string `json:"ext"`            // extension without the dot, e.g. "md"
	Name string `json:"name"`           // shown by the system, e.g. "Markdown document"
	Role string `json:"role,omitempty"` // "editor" (default) or "viewer"
	MIME string `json:"mime,omitempty"` // default application/x-<exe>-<ext>
}

// Viewer reports whether the app only views these files.
func (f FileAssociation) Viewer() bool { return f.Role == "viewer" }

// MIMEType is MIME, or a private type derived from the app and extension.
func (f FileAssociation) MIMEType(c *Config) string {
	if f.MIME != "" {
		return f.MIME
	}
	return "application/x-" + strings.ToLower(c.Exe()) + "-" + f.Ext
}

// MacOS holds macOS-only settings.
type MacOS struct {
	MinVersion   string `json:"minVersion,omitempty"`   // LSMinimumSystemVersion; default 11.0
	Entitlements string `json:"entitlements,omitempty"` // plist used by tan sign
}

// Windows holds Windows-only settings. See docs/windows.md.
type Windows struct {
	// Code signing (tan sign, tan package): a PKCS#12 (.pfx) file, unless
	// -cert or $TANGO_WINDOWS_CERT names another. The password is never
	// read from tan.json: it comes from $TANGO_WINDOWS_CERT_PASSWORD.
	Certificate  string `json:"certificate,omitempty"`
	TimestampURL string `json:"timestampURL,omitempty"` // RFC 3161 server; default http://timestamp.digicert.com
	// Publisher is the certificate's subject, e.g. "CN=Example Ltd, O=Example Ltd, C=US";
	// MSIX packages need it (it must match the signing certificate).
	Publisher string `json:"publisher,omitempty"`
}

// Linux holds the settings of the Linux packages tan package makes with
// -format deb, rpm or flatpak. See docs/linux-packaging.md.
type Linux struct {
	// Package is the deb and rpm package name; default the name in lower
	// case with words joined by '-' ("My App" → "my-app").
	Package string `json:"package,omitempty"`
	// Maintainer is the Debian Maintainer and rpm Packager, "Name <email>".
	Maintainer string `json:"maintainer,omitempty"`
	// Description: the first line is the one-line summary, the rest the
	// long description. Default the app name.
	Description string `json:"description,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	License     string `json:"license,omitempty"` // rpm License; default "Proprietary"
	// Depends and RPMRequires are added to the defaults: GTK 3 and
	// WebKitGTK 4.1, which the Tango runtime loads (libgtk-3-0 and
	// libwebkit2gtk-4.1-0 for deb, their sonames for rpm).
	Depends     []string `json:"depends,omitempty"`     // e.g. "xdg-utils" or "libfoo1 (>= 1.2)"
	RPMRequires []string `json:"rpmRequires,omitempty"` // e.g. "xdg-utils"
	Flatpak     Flatpak  `json:"flatpak,omitzero"`
}

// Flatpak holds the settings of the Flatpak manifest.
type Flatpak struct {
	RuntimeVersion string `json:"runtimeVersion,omitempty"` // org.gnome.Platform branch; default defaultFlatpakRuntime
	// FinishArgs are sandbox permissions added to the defaults (IPC,
	// Wayland, X11 fallback, GPU, network), e.g. "--socket=pulseaudio" or
	// "--filesystem=xdg-documents".
	FinishArgs []string `json:"finishArgs,omitempty"`
}

// IOS holds iOS-only settings.
type IOS struct {
	MinVersion string `json:"minVersion,omitempty"` // MinimumOSVersion; default 17.0
	Build      string `json:"build,omitempty"`      // CFBundleVersion; default the version without its suffix
	// Device builds: the codesign identity (or $TANGO_IOS_SIGN_IDENTITY),
	// the provisioning profile, and entitlements (default: the ones the
	// profile grants).
	SignIdentity        string `json:"signIdentity,omitempty"`
	ProvisioningProfile string `json:"provisioningProfile,omitempty"`
	Entitlements        string `json:"entitlements,omitempty"`
	// UsageDescriptions overrides the text iOS shows when it asks for a
	// permission, by permission name ("camera": "Scan receipts").
	UsageDescriptions map[string]string `json:"usageDescriptions,omitempty"`
}

// Android holds Android-only settings.
type Android struct {
	MinSdk      int `json:"minSdk,omitempty"`      // default 24 (the lowest the host supports)
	TargetSdk   int `json:"targetSdk,omitempty"`   // default 36
	VersionCode int `json:"versionCode,omitempty"` // default derived from version: 1.2.3 → 10203
	// Release signing. The passwords come from $TANGO_ANDROID_KEYSTORE_PASSWORD
	// and $TANGO_ANDROID_KEY_PASSWORD (default: the keystore password).
	Keystore string `json:"keystore,omitempty"`
	KeyAlias string `json:"keyAlias,omitempty"`
}

const (
	defaultIOSMin       = "17.0"
	androidHostMinSdk   = 24 // the Java host's lowest API level
	defaultAndroidSdk   = 36 // targetSdk and compileSdk
	envIOSSignIdentity  = "TANGO_IOS_SIGN_IDENTITY"
	envAndroidStorePass = "TANGO_ANDROID_KEYSTORE_PASSWORD"
	envAndroidKeyPass   = "TANGO_ANDROID_KEY_PASSWORD"
)

var (
	iosVersionPattern = regexp.MustCompile(`^\d+(\.\d+){0,2}$`)
	idPattern         = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
	schemePattern     = regexp.MustCompile(`^[a-z][a-z0-9+.-]*$`) // RFC 3986 scheme, lower case
	extPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
	mimePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`)
	versionPattern    = regexp.MustCompile(`^\d+(\.\d+){0,2}([-+][0-9A-Za-z.-]+)?$`)
	// keyAliasPattern: a keytool alias tan passes to Gradle; nothing a
	// shell or cmd.exe would interpret.
	keyAliasPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

// ParseConfig decodes and validates tan.json. Unknown fields are errors so
// typos do not pass silently.
func ParseConfig(data []byte) (*Config, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c Config
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	return &c, nil
}

func (c *Config) validate() error {
	var errs []error
	if strings.TrimSpace(c.Name) == "" {
		errs = append(errs, errors.New(`"name" is required`))
	} else if c.Exe() == "" {
		errs = append(errs, fmt.Errorf(`"name" %q has no letters or digits`, c.Name))
	} else if err := checkAppName(c.Name); err != nil {
		errs = append(errs, err)
	}
	if !idPattern.MatchString(c.ID) {
		errs = append(errs, fmt.Errorf(`"id" %q must be reverse-DNS, like com.example.app`, c.ID))
	}
	if !versionPattern.MatchString(c.Version) {
		errs = append(errs, fmt.Errorf(`"version" %q must look like 1.2.3`, c.Version))
	}
	if c.HasFrontend() { // no "frontend" at all: a self-drawn UI app (package uihost)
		if c.Frontend.Dir == "" {
			errs = append(errs, errors.New(`"frontend.dir" is required`))
		}
		if c.Frontend.Dist == "" {
			errs = append(errs, errors.New(`"frontend.dist" is required (use "." to serve frontend.dir as is)`))
		}
	}
	reserved := []string{"http", "https", "file", "mailto", "ftp", "data", "javascript", "about"}
	for _, sc := range c.URLSchemes {
		if !schemePattern.MatchString(sc) || slices.Contains(reserved, sc) {
			errs = append(errs, fmt.Errorf(`"urlSchemes": %q is not a lower-case custom URL scheme`, sc))
		}
	}
	seen := map[string]bool{}
	for _, f := range c.FileAssociations {
		ext := strings.ToLower(f.Ext)
		switch {
		case !extPattern.MatchString(f.Ext):
			errs = append(errs, fmt.Errorf(`"fileAssociations": extension %q must have no dot and only letters, digits, '-' or '_'`, f.Ext))
		case seen[ext]:
			errs = append(errs, fmt.Errorf(`"fileAssociations": extension %q listed twice`, f.Ext))
		}
		seen[ext] = true
		if strings.TrimSpace(f.Name) == "" {
			errs = append(errs, fmt.Errorf(`"fileAssociations": %q needs a "name"`, f.Ext))
		}
		if f.Role != "" && f.Role != "editor" && f.Role != "viewer" {
			errs = append(errs, fmt.Errorf(`"fileAssociations": role %q must be "editor" or "viewer"`, f.Role))
		}
		if f.MIME != "" && !mimePattern.MatchString(f.MIME) {
			errs = append(errs, fmt.Errorf(`"fileAssociations": %q is not a MIME type`, f.MIME))
		}
	}
	errs = append(errs, c.validateMobile()...)
	errs = append(errs, c.validateAppLinks()...)
	errs = append(errs, c.Icons.validate()...)
	errs = append(errs, c.validateWindows()...)
	errs = append(errs, c.validateLinux()...)
	errs = append(errs, c.Installer.validate()...)
	for _, p := range []string{c.Icon, c.Frontend.Dir, c.Frontend.Dist, c.MacOS.Entitlements,
		c.IOS.ProvisioningProfile, c.IOS.Entitlements, c.Android.Keystore, c.Windows.Certificate} {
		if configPathAbs(p) {
			errs = append(errs, fmt.Errorf("path %q must be relative to the project", p))
		}
	}
	return errors.Join(errs...)
}

// checkAppName rejects names that would break the files tan generates
// from them: control characters (a newline would add lines to a .desktop
// file or a deb control file), path separators and ".." (the macOS bundle
// is <name>.app in the build directory).
func checkAppName(name string) error {
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf(`"name" %q must not contain control characters such as newlines`, name)
		}
	}
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return fmt.Errorf(`"name" %q must not contain '/', '\' or ".."`, name)
	}
	return nil
}

func (c *Config) validateWindows() []error {
	var errs []error
	w := c.Windows
	if w.TimestampURL != "" && !strings.HasPrefix(w.TimestampURL, "http://") && !strings.HasPrefix(w.TimestampURL, "https://") {
		errs = append(errs, fmt.Errorf(`"windows.timestampURL" %q must be an http(s) URL`, w.TimestampURL))
	}
	if w.Publisher != "" && !strings.HasPrefix(w.Publisher, "CN=") {
		errs = append(errs, fmt.Errorf(`"windows.publisher" %q must be a certificate subject starting with CN=`, w.Publisher))
	}
	if ext := strings.ToLower(filepath.Ext(w.Certificate)); w.Certificate != "" && ext != ".pfx" && ext != ".p12" {
		errs = append(errs, fmt.Errorf(`"windows.certificate" %q must be a .pfx or .p12 file`, w.Certificate))
	}
	return errs
}

var (
	linuxPackagePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`) // Debian Policy 5.6.1
	linuxDependPattern  = regexp.MustCompile(`^[^,\n]+$`)
)

func (c *Config) validateLinux() []error {
	var errs []error
	l := c.Linux
	if l.Package != "" && !linuxPackagePattern.MatchString(l.Package) {
		errs = append(errs, fmt.Errorf(`"linux.package" %q must be lower-case letters, digits, '+', '-' or '.', at least two characters, starting with a letter or digit`, l.Package))
	}
	for name, v := range map[string]string{"maintainer": l.Maintainer, "homepage": l.Homepage, "license": l.License, "flatpak.runtimeVersion": l.Flatpak.RuntimeVersion} {
		if strings.ContainsAny(v, "\n\r") {
			errs = append(errs, fmt.Errorf(`"linux.%s" must be one line`, name))
		}
	}
	for _, d := range append(slices.Clip(l.Depends), l.RPMRequires...) {
		if !linuxDependPattern.MatchString(d) || strings.TrimSpace(d) == "" {
			errs = append(errs, fmt.Errorf(`"linux.depends"/"linux.rpmRequires": %q must be one dependency, without commas`, d))
		}
	}
	for _, a := range l.Flatpak.FinishArgs {
		if !strings.HasPrefix(a, "--") {
			errs = append(errs, fmt.Errorf(`"linux.flatpak.finishArgs": %q must be a flatpak option like --socket=pulseaudio`, a))
		}
	}
	return errs
}

func (c *Config) validateMobile() []error {
	var errs []error
	seen := map[string]bool{}
	canonical := map[string]bool{}
	for _, p := range c.Permissions {
		perm := lookupPermission(p)
		switch {
		case perm == nil:
			errs = append(errs, fmt.Errorf(`"permissions": unknown permission %q (known: %s)`, p, strings.Join(permissionNames(), ", ")))
		case canonical[perm.name]:
			errs = append(errs, fmt.Errorf(`"permissions": %q listed twice`, p))
		default:
			canonical[perm.name] = true
		}
		seen[p] = true
	}
	ios := c.IOS
	if ios.MinVersion != "" && !iosVersionPattern.MatchString(ios.MinVersion) {
		errs = append(errs, fmt.Errorf(`"ios.minVersion" %q must look like 17.0`, ios.MinVersion))
	}
	if ios.Build != "" && !iosVersionPattern.MatchString(ios.Build) {
		errs = append(errs, fmt.Errorf(`"ios.build" %q must be numbers separated by dots, like 42 or 1.2.3`, ios.Build))
	}
	for _, name := range sortedKeys(ios.UsageDescriptions) {
		p := lookupPermission(name)
		switch {
		case p == nil:
			errs = append(errs, fmt.Errorf(`"ios.usageDescriptions": unknown permission %q`, name))
		case !seen[name]:
			errs = append(errs, fmt.Errorf(`"ios.usageDescriptions": %q is not in "permissions"`, name))
		case len(p.iosKeys) == 0:
			errs = append(errs, fmt.Errorf(`"ios.usageDescriptions": iOS shows no usage description for %q`, name))
		case strings.TrimSpace(ios.UsageDescriptions[name]) == "":
			errs = append(errs, fmt.Errorf(`"ios.usageDescriptions": %q is empty`, name))
		}
	}
	a := c.Android
	if a.MinSdk != 0 && a.MinSdk < androidHostMinSdk {
		errs = append(errs, fmt.Errorf(`"android.minSdk" %d is below %d, the lowest the Tango host supports`, a.MinSdk, androidHostMinSdk))
	}
	if a.TargetSdk != 0 && a.TargetSdk < c.AndroidMinSdk() {
		errs = append(errs, fmt.Errorf(`"android.targetSdk" %d is below minSdk %d`, a.TargetSdk, c.AndroidMinSdk()))
	}
	if a.VersionCode < 0 || a.VersionCode > 2100000000 {
		errs = append(errs, fmt.Errorf(`"android.versionCode" %d must be between 1 and 2100000000`, a.VersionCode))
	}
	if (a.Keystore == "") != (a.KeyAlias == "") {
		errs = append(errs, errors.New(`"android.keystore" and "android.keyAlias" go together`))
	}
	if a.KeyAlias != "" && !keyAliasPattern.MatchString(a.KeyAlias) {
		errs = append(errs, fmt.Errorf(`"android.keyAlias" %q must be letters, digits, '.', '_' or '-'`, a.KeyAlias))
	}
	return errs
}

// IOSMinVersion is ios.minVersion or the default.
func (c *Config) IOSMinVersion() string { return cmp.Or(c.IOS.MinVersion, defaultIOSMin) }

// IOSShortVersion is CFBundleShortVersionString: the version without a
// pre-release or build suffix, which iOS does not accept.
func (c *Config) IOSShortVersion() string {
	v, _, _ := strings.Cut(c.Version, "-")
	v, _, _ = strings.Cut(v, "+")
	return v
}

// IOSBuild is CFBundleVersion.
func (c *Config) IOSBuild() string { return cmp.Or(c.IOS.Build, c.IOSShortVersion()) }

// AndroidMinSdk and AndroidTargetSdk are the configured API levels or the
// defaults.
func (c *Config) AndroidMinSdk() int    { return cmp.Or(c.Android.MinSdk, androidHostMinSdk) }
func (c *Config) AndroidTargetSdk() int { return cmp.Or(c.Android.TargetSdk, defaultAndroidSdk) }

// AndroidVersionCode is android.versionCode, or one derived from the
// version so that it grows with it: major*10000 + minor*100 + patch.
func (c *Config) AndroidVersionCode() int {
	if c.Android.VersionCode > 0 {
		return c.Android.VersionCode
	}
	parts := strings.Split(c.IOSShortVersion(), ".")
	code := 0
	for i := range 3 {
		code *= 100
		if i < len(parts) {
			n, _ := strconv.Atoi(parts[i])
			if i > 0 {
				n = min(n, 99)
			}
			code += n
		}
	}
	return max(code, 1)
}

// LoadConfig reads tan.json from dir.
func LoadConfig(dir string) (*Config, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(abs, ConfigFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("no %s in %s (run tan init, or pass -C)", ConfigFile, abs)
		}
		return nil, err
	}
	c, err := ParseConfig(data)
	if err != nil {
		return nil, err
	}
	c.dir = abs
	return c, nil
}

// Path resolves a project-relative path.
func (c *Config) Path(rel string) string { return filepath.Join(c.dir, rel) }

// configPathAbs reports whether a tan.json path is absolute on any OS: a
// leading / or \ or a drive letter. tan.json is shared between machines,
// so it is judged the same everywhere (filepath.IsAbs accepts /abs as
// relative on Windows and C:\abs on Unix).
func configPathAbs(p string) bool {
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return true
	}
	return len(p) >= 2 && p[1] == ':' && ('a' <= p[0]|0x20 && p[0]|0x20 <= 'z')
}

// HasFrontend reports whether tan.json has a "frontend" section. Apps whose
// UI is drawn by package ui (shown with uihost) leave it out: tan then
// skips every front-end step.
func (c *Config) HasFrontend() bool { return c.Frontend != Frontend{} }

// FrontendDir is the absolute front-end directory ("" without a front end).
func (c *Config) FrontendDir() string {
	if !c.HasFrontend() {
		return ""
	}
	return c.Path(c.Frontend.Dir)
}

// DistDir is the absolute front-end build output directory.
func (c *Config) DistDir() string { return filepath.Join(c.FrontendDir(), c.Frontend.Dist) }

// Exe is the executable name: Name with everything but letters, digits,
// '-' and '_' removed ("My App" → "MyApp").
func (c *Config) Exe() string {
	var b strings.Builder
	for _, r := range c.Name {
		if r == '-' || r == '_' || r < 128 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Marshal encodes c the way init writes it.
func (c *Config) Marshal() []byte {
	b, _ := json.MarshalIndent(c, "", "  ")
	return append(b, '\n')
}

// slug lowercases s and joins its words with '-' ("My App" → "my-app").
func slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	return b.String()
}

// AppleLocalizations returns CFBundleLocalizations for macOS and iOS.
func (c *Config) AppleLocalizations() []string {
	if len(c.Localizations) > 0 {
		return c.Localizations
	}
	return macLocalizations
}

// Installer is the "installer" section of tan.json: how tan package
// dresses its installers. Every field is optional. See docs/packaging.md.
//
//	"installer": {
//	  "windows": {"scope": "user", "license": "LICENSE", "sidebarImage": "build/installer/sidebar.png"},
//	  "macos": {"dmg": {"background": "build/dmg-background.png"}}
//	}
type Installer struct {
	Windows NSISInstaller `json:"windows,omitzero"`
	MacOS   MacInstaller  `json:"macos,omitzero"`
}

// NSISInstaller customises the Windows NSIS installer (package_nsis.go).
type NSISInstaller struct {
	// Scope is "user" (default: %LOCALAPPDATA%\Programs, HKCU, no UAC
	// prompt, and the app can update itself) or "machine" (Program Files,
	// HKLM, asks for administrator rights).
	Scope string `json:"scope,omitempty"`
	// License is a .txt or .rtf file shown on a licence page the user
	// must accept.
	License string `json:"license,omitempty"`
	// Welcome shows the welcome page; default true.
	Welcome *bool `json:"welcome,omitempty"`
	// SidebarImage is the welcome and finish pages' left image (164×314,
	// .bmp, or .png that tan converts); HeaderImage the banner on the
	// other pages (150×57). Default: NSIS's own.
	SidebarImage string `json:"sidebarImage,omitempty"`
	HeaderImage  string `json:"headerImage,omitempty"`
	// RunAfterInstall offers "Run <app>" on the finish page; default true
	// for a per-user installer, false for scope "machine" (an elevated
	// installer). Turned on there, the app is started through Explorer so
	// it does not inherit the installer's administrator token.
	RunAfterInstall *bool `json:"runAfterInstall,omitempty"`
	// DesktopShortcut is "ask" (default: a check box on the finish page),
	// "always" or "never".
	DesktopShortcut string `json:"desktopShortcut,omitempty"`
	// LanguageDialog asks for the installer language at start; by default
	// the installer follows the Windows display language.
	LanguageDialog bool `json:"languageDialog,omitempty"`
	// Include is an NSIS file included in the script; the macros it
	// defines named customInit, customInstall and customUnInstall run in
	// .onInit, at the end of the install section and at the start of the
	// uninstall section.
	Include string `json:"include,omitempty"`
	// Template is a Go text/template file that replaces the whole
	// generated script (variables: docs/packaging.md).
	Template string `json:"template,omitempty"`
}

// MacInstaller customises the macOS disk image (package_dmg.go).
type MacInstaller struct {
	DMG DMGLayout `json:"dmg,omitzero"`
}

// DMGLayout is the Finder window a disk image opens in. Sizes and
// positions are in points; positions are icon centres from the window's
// top left corner.
type DMGLayout struct {
	// Background is a PNG, JPEG or TIFF drawn at the top left of the
	// window at its size in points (a 72 dpi image: pixels; a file named
	// x@2x.png next to x.png makes a Retina background).
	Background           string `json:"background,omitempty"`
	WindowSize           []int  `json:"windowSize,omitempty"`           // [width, height]; default [540, 380]
	IconSize             int    `json:"iconSize,omitempty"`             // default 128
	AppPosition          []int  `json:"appPosition,omitempty"`          // default [140, 190]
	ApplicationsPosition []int  `json:"applicationsPosition,omitempty"` // default [400, 190]
	Title                string `json:"title,omitempty"`                // volume name; default the app name
}

func (in Installer) validate() []error {
	var errs []error
	w := in.Windows
	if w.Scope != "" && w.Scope != "user" && w.Scope != "machine" {
		errs = append(errs, fmt.Errorf(`"installer.windows.scope" %q must be "user" or "machine"`, w.Scope))
	}
	switch w.DesktopShortcut {
	case "", "ask", "always", "never":
	default:
		errs = append(errs, fmt.Errorf(`"installer.windows.desktopShortcut" %q must be "ask", "always" or "never"`, w.DesktopShortcut))
	}
	if ext := strings.ToLower(filepath.Ext(w.License)); w.License != "" && ext != "" && ext != ".txt" && ext != ".rtf" && ext != ".md" {
		errs = append(errs, fmt.Errorf(`"installer.windows.license" %q must be a plain-text or .rtf file`, w.License))
	}
	for name, p := range map[string]string{"sidebarImage": w.SidebarImage, "headerImage": w.HeaderImage} {
		if ext := strings.ToLower(filepath.Ext(p)); p != "" && ext != ".bmp" && ext != ".png" {
			errs = append(errs, fmt.Errorf(`"installer.windows.%s" %q must be a .bmp or .png file`, name, p))
		}
	}
	d := in.MacOS.DMG
	if ext := strings.ToLower(filepath.Ext(d.Background)); d.Background != "" && !slices.Contains([]string{".png", ".jpg", ".jpeg", ".tif", ".tiff"}, ext) {
		errs = append(errs, fmt.Errorf(`"installer.macos.dmg.background" %q must be a PNG, JPEG or TIFF file`, d.Background))
	}
	pair := func(name string, v []int, lo int) {
		if v != nil && (len(v) != 2 || v[0] < lo || v[1] < lo || v[0] > 10000 || v[1] > 10000) {
			errs = append(errs, fmt.Errorf(`"installer.macos.dmg.%s" %v must be two numbers from %d to 10000`, name, v, lo))
		}
	}
	pair("windowSize", d.WindowSize, 100)
	pair("appPosition", d.AppPosition, 0)
	pair("applicationsPosition", d.ApplicationsPosition, 0)
	if d.IconSize != 0 && (d.IconSize < 16 || d.IconSize > 512) {
		errs = append(errs, fmt.Errorf(`"installer.macos.dmg.iconSize" %d must be from 16 to 512`, d.IconSize))
	}
	if strings.ContainsAny(d.Title, ":/\n") || len(d.Title) > 255 {
		errs = append(errs, fmt.Errorf(`"installer.macos.dmg.title" %q must be one line without ':' or '/'`, d.Title))
	}
	for _, p := range []string{w.License, w.SidebarImage, w.HeaderImage, w.Include, w.Template, d.Background} {
		if configPathAbs(p) {
			errs = append(errs, fmt.Errorf("path %q must be relative to the project", p))
		}
	}
	return errs
}
