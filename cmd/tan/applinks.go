package main

// iOS Universal Links and Android App Links: tan.json "appLinks" and
// "appLinkPaths", the associated-domains entitlement, the autoVerify
// intent filters, and tan applinks, which writes the two files the web
// server must host under /.well-known/.
//
// References: Apple "Supporting associated domains" and "Associated
// Domains Entitlement"; Android "Add Android App Links" and "Verify
// Android App Links".

import (
	"bytes"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// associatedDomainsKey is the iOS entitlement listing "applinks:<domain>".
const associatedDomainsKey = "com.apple.developer.associated-domains"

var (
	// appLinkDomainPattern is a lower-case DNS name with at least two
	// labels, optionally behind "*." (all subdomains).
	appLinkDomainPattern = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	appLinkPathPattern   = regexp.MustCompile(`^/[A-Za-z0-9\-._~%/]*$`) // no wildcards: "*" and "?" mean something in apple-app-site-association
	appleTeamPattern     = regexp.MustCompile(`^[A-Z0-9]{10}$`)
)

// validateAppLinks checks "appLinks" and "appLinkPaths".
func (c *Config) validateAppLinks() []error {
	var errs []error
	seen := map[string]bool{}
	for _, d := range c.AppLinks {
		switch {
		case strings.Contains(d, "://"):
			errs = append(errs, fmt.Errorf(`"appLinks": %q must be a bare domain without a scheme, like example.com`, d))
		case strings.ContainsAny(d, "/?#"):
			errs = append(errs, fmt.Errorf(`"appLinks": %q must be a bare domain without a path (paths go in "appLinkPaths")`, d))
		case strings.Contains(d, ":"):
			errs = append(errs, fmt.Errorf(`"appLinks": %q must not have a port (App Links are verified over https on port 443)`, d))
		case d != strings.ToLower(d):
			errs = append(errs, fmt.Errorf(`"appLinks": %q must be lower case`, d))
		case !appLinkDomainPattern.MatchString(d):
			errs = append(errs, fmt.Errorf(`"appLinks": %q is not a domain like example.com or *.example.com`, d))
		case seen[d]:
			errs = append(errs, fmt.Errorf(`"appLinks": %q listed twice`, d))
		}
		seen[d] = true
	}
	if len(c.AppLinkPaths) > 0 && len(c.AppLinks) == 0 {
		errs = append(errs, errors.New(`"appLinkPaths" needs "appLinks"`))
	}
	for _, p := range c.AppLinkPaths {
		if !appLinkPathPattern.MatchString(p) {
			errs = append(errs, fmt.Errorf(`"appLinkPaths": %q must be a URL path starting with "/" (letters, digits, "-", ".", "_", "~", "%%", "/")`, p))
		}
	}
	return errs
}

// appLinkHost is the host that serves a domain's /.well-known files: both
// systems look up *.example.com at example.com.
func appLinkHost(domain string) string { return strings.TrimPrefix(domain, "*.") }

// associatedDomains are the entitlement values for tan.json "appLinks".
func (c *Config) associatedDomains() []string {
	out := make([]string, len(c.AppLinks))
	for i, d := range c.AppLinks {
		out[i] = "applinks:" + d
	}
	return out
}

// withAssociatedDomains returns ent with the app's applinks: entries in
// com.apple.developer.associated-domains, keeping the other entries
// (webcredentials:, a hand-written applinks:...). A provisioning profile
// grants the capability as the string "*", which is replaced.
func withAssociatedDomains(ent plist, domains []string) plist {
	out := slices.Clone(ent)
	var vals []any
	seen := map[string]bool{}
	add := func(s string) {
		if s != "*" && !seen[s] {
			seen[s] = true
			vals = append(vals, s)
		}
	}
	switch old := out.get(associatedDomainsKey).(type) {
	case []any:
		for _, v := range old {
			if s, ok := v.(string); ok {
				add(s)
			}
		}
	case []string:
		for _, s := range old {
			add(s)
		}
	}
	for _, d := range domains {
		add(d)
	}
	out.set(associatedDomainsKey, vals)
	return out
}

// readPlistFile reads an XML or binary property list (binary ones are
// converted with plutil, which device builds have).
func readPlistFile(ctx context.Context, path string) (plist, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(data, []byte("bplist")) {
		if data, err = exec.CommandContext(ctx, "plutil", "-convert", "xml1", "-o", "-", path).Output(); err != nil {
			return nil, fmt.Errorf("%s: plutil -convert xml1: %v", path, err)
		}
	}
	p, err := decodePlist(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// writeAppLinkEntitlements writes base (an entitlements plist, or "" for
// none) plus the app's associated domains to out.
func writeAppLinkEntitlements(ctx context.Context, cfg *Config, base, out string) error {
	var ent plist
	if base != "" {
		var err error
		if ent, err = readPlistFile(ctx, base); err != nil {
			return err
		}
	}
	return os.WriteFile(out, encodePlist(withAssociatedDomains(ent, cfg.associatedDomains())), 0o644)
}

// readProfile decodes a .mobileprovision (a CMS-signed plist).
func readProfile(ctx context.Context, profile string) (plist, error) {
	out, err := exec.CommandContext(ctx, "security", "cms", "-D", "-i", profile).Output()
	if err != nil {
		return nil, fmt.Errorf("reading provisioning profile %s: %v", profile, err)
	}
	p, err := decodePlist(out)
	if err != nil {
		return nil, fmt.Errorf("provisioning profile %s: %w", profile, err)
	}
	return p, nil
}

// profileHasAssociatedDomains reports whether a decoded profile grants
// the Associated Domains capability.
func profileHasAssociatedDomains(profile plist) bool {
	ent, _ := profile.get("Entitlements").(plist)
	return ent.get(associatedDomainsKey) != nil
}

// profileTeam is the profile's team identifier.
func profileTeam(profile plist) string {
	if ids, ok := profile.get("TeamIdentifier").([]any); ok && len(ids) > 0 {
		if s, ok := ids[0].(string); ok {
			return s
		}
	}
	ent, _ := profile.get("Entitlements").(plist)
	s, _ := ent.get("com.apple.developer.team-identifier").(string)
	return s
}

// warnProfileAppLinks warns when the profile cannot sign the
// associated-domains entitlement: iOS refuses to install such an app.
func warnProfileAppLinks(ctx context.Context, cfg *Config, profile string) {
	p, err := readProfile(ctx, profile)
	if err != nil {
		warnf("%v", err)
		return
	}
	if !profileHasAssociatedDomains(p) {
		warnf("tan.json \"appLinks\" needs the Associated Domains capability, but provisioning profile %s does not grant %s: "+
			"iOS will refuse to install the app. Enable Associated Domains for the App ID %s at developer.apple.com "+
			"(Certificates, Identifiers & Profiles), then download the profile again", cfg.IOS.ProvisioningProfile, associatedDomainsKey, cfg.ID)
	}
}

// androidAppLinkFilters adds an autoVerify intent filter for each
// "appLinks" domain to the host Activity, after the deep-link filter. A
// domain whose host the Activity already declares is skipped, so running
// it again changes nothing.
func androidAppLinkFilters(manifest string, cfg *Config) (string, error) {
	if len(cfg.AppLinks) == 0 {
		return manifest, nil
	}
	act := strings.Index(manifest, `"`+hostActivity+`"`)
	if act < 0 {
		return "", fmt.Errorf("AndroidManifest.xml template does not declare %s", hostActivity)
	}
	end := strings.Index(manifest[act:], "</activity>")
	if end < 0 {
		return "", fmt.Errorf("AndroidManifest.xml: %s has no </activity>", hostActivity)
	}
	end += act
	activity := manifest[act:end]
	var f strings.Builder
	for _, d := range cfg.AppLinks {
		if strings.Contains(activity, `android:host="`+d+`"`) {
			continue
		}
		fmt.Fprintf(&f, "    <!-- App Links: tan.json \"appLinks\"; verified with https://%s/.well-known/assetlinks.json. -->\n", appLinkHost(d))
		f.WriteString("            <intent-filter android:autoVerify=\"true\">\n")
		f.WriteString("                <action android:name=\"android.intent.action.VIEW\" />\n")
		f.WriteString("                <category android:name=\"android.intent.category.DEFAULT\" />\n")
		f.WriteString("                <category android:name=\"android.intent.category.BROWSABLE\" />\n")
		f.WriteString("                <data android:scheme=\"https\" />\n")
		f.WriteString("                <data android:scheme=\"http\" />\n")
		fmt.Fprintf(&f, "                <data android:host=\"%s\" />\n", d)
		for _, p := range cfg.AppLinkPaths {
			fmt.Fprintf(&f, "                <data android:pathPrefix=\"%s\" />\n", p)
		}
		f.WriteString("            </intent-filter>\n        ")
	}
	return manifest[:end] + f.String() + manifest[end:], nil
}

// appleAppSiteAssociation is /.well-known/apple-app-site-association in
// the iOS 13+ format (components; Tango apps need iOS 17).
func appleAppSiteAssociation(appIDs, paths []string) ([]byte, error) {
	type component struct {
		Path string `json:"/"`
	}
	type detail struct {
		AppIDs     []string    `json:"appIDs"`
		Components []component `json:"components"`
	}
	d := detail{AppIDs: appIDs}
	for _, p := range paths {
		d.Components = append(d.Components, component{p + "*"})
	}
	if len(d.Components) == 0 {
		d.Components = []component{{"*"}}
	}
	doc := map[string]any{"applinks": map[string]any{"details": []detail{d}}}
	b, err := json.MarshalIndent(doc, "", "  ")
	return append(b, '\n'), err
}

// assetLinks is /.well-known/assetlinks.json (Digital Asset Links).
func assetLinks(pkg string, fingerprints []string) ([]byte, error) {
	type target struct {
		Namespace    string   `json:"namespace"`
		PackageName  string   `json:"package_name"`
		Fingerprints []string `json:"sha256_cert_fingerprints"`
	}
	type statement struct {
		Relation []string `json:"relation"`
		Target   target   `json:"target"`
	}
	doc := []statement{{
		Relation: []string{"delegate_permission/common.handle_all_urls"},
		Target:   target{Namespace: "android_app", PackageName: pkg, Fingerprints: fingerprints},
	}}
	b, err := json.MarshalIndent(doc, "", "  ")
	return append(b, '\n'), err
}

// normalizeFingerprint turns a SHA-256 certificate fingerprint, with or
// without colons, into the upper-case colon-separated form assetlinks.json
// uses.
func normalizeFingerprint(s string) (string, error) {
	hex := strings.ToUpper(strings.NewReplacer(":", "", " ", "").Replace(strings.TrimSpace(s)))
	if len(hex) != 64 || strings.Trim(hex, "0123456789ABCDEF") != "" {
		return "", fmt.Errorf("%q is not a SHA-256 fingerprint (64 hex digits)", s)
	}
	var b strings.Builder
	for i := 0; i < 64; i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(hex[i : i+2])
	}
	return b.String(), nil
}

// keytoolFingerprint reads the SHA-256 fingerprint of alias's certificate
// in a keystore.
func keytoolFingerprint(ctx context.Context, keystore, alias, password string) (string, error) {
	keytool := "keytool"
	if home := os.Getenv("JAVA_HOME"); home != "" {
		keytool = filepath.Join(home, "bin", exeName("keytool"))
	}
	cmd := exec.CommandContext(ctx, keytool, "-J-Duser.language=en", "-list", "-v", "-keystore", keystore, "-alias", alias, "-storepass:env", "TAN_KEYTOOL_PASS")
	cmd.Env = append(os.Environ(), "TAN_KEYTOOL_PASS="+password)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("keytool -list %s: %v: %s", keystore, err, bytes.TrimSpace(out))
	}
	fp := keytoolSHA256(string(out))
	if fp == "" {
		return "", fmt.Errorf("keytool printed no SHA256 fingerprint for %s in %s", alias, keystore)
	}
	return normalizeFingerprint(fp)
}

// keytoolSHA256 finds "SHA256: AB:CD:..." in keytool -list -v output.
func keytoolSHA256(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "SHA256:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// debugKeystore is where the Android Gradle plugin keeps the debug key.
func debugKeystore() string {
	if v := os.Getenv("ANDROID_USER_HOME"); v != "" {
		return filepath.Join(v, "debug.keystore")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".android", "debug.keystore")
}

// signingTeam finds the Apple team: the provisioning profile's, else the
// OU of the signing certificate (the team id on Apple certificates).
func signingTeam(ctx context.Context, cfg *Config) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("reading the team from the provisioning profile or keychain needs macOS")
	}
	if cfg.IOS.ProvisioningProfile != "" {
		p, err := readProfile(ctx, cfg.Path(cfg.IOS.ProvisioningProfile))
		if err != nil {
			return "", err
		}
		if t := profileTeam(p); t != "" {
			return t, nil
		}
	}
	identity := firstNonEmpty(os.Getenv(envIOSSignIdentity), cfg.IOS.SignIdentity)
	if identity == "" {
		return "", errors.New(`tan.json has neither "ios.provisioningProfile" nor "ios.signIdentity"`)
	}
	out, err := exec.CommandContext(ctx, "security", "find-certificate", "-c", identity, "-p").Output()
	if err != nil {
		return "", fmt.Errorf("certificate %q not found in the keychain", identity)
	}
	blk, _ := pem.Decode(out)
	if blk == nil {
		return "", fmt.Errorf("certificate %q: no PEM data", identity)
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return "", fmt.Errorf("certificate %q: %w", identity, err)
	}
	if len(cert.Subject.OrganizationalUnit) == 0 {
		return "", fmt.Errorf("certificate %q has no team (OU)", identity)
	}
	return cert.Subject.OrganizationalUnit[0], nil
}

// stringsFlag is a repeatable string flag.
type stringsFlag []string

func (s *stringsFlag) String() string     { return strings.Join(*s, ",") }
func (s *stringsFlag) Set(v string) error { *s = append(*s, v); return nil }

var _ flag.Value = (*stringsFlag)(nil)

func runAppLinks(args []string) error {
	fs := newFlags("applinks", "[flags]")
	dir := projectFlag(fs)
	out := fs.String("o", "build/applinks", "output `dir`; the files go in <dir>/.well-known")
	platform := fs.String("platform", "ios,android", "which files: ios (apple-app-site-association), android (assetlinks.json) or both")
	team := fs.String("team", "", "Apple team `ID` (default: from ios.provisioningProfile or ios.signIdentity)")
	var fps stringsFlag
	fs.Var(&fps, "fingerprint", "SHA-256 signing certificate `fingerprint` for assetlinks.json; repeat for several keys (Play App Signing: the app signing key from Play Console). Default: android.keystore's key")
	debug := fs.Bool("debug", false, "android: use the debug keystore's fingerprint (for debug builds only)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := noArgs(fs); err != nil {
		return err
	}
	cfg, err := LoadConfig(*dir)
	if err != nil {
		return err
	}
	if len(cfg.AppLinks) == 0 {
		return errors.New(`tan.json has no "appLinks"; add the domains first, e.g. "appLinks": ["example.com"]`)
	}
	var ios, android bool
	for _, p := range strings.Split(*platform, ",") {
		switch strings.TrimSpace(p) {
		case "ios":
			ios = true
		case "android":
			android = true
		default:
			return fmt.Errorf("-platform: unknown %q (ios, android)", p)
		}
	}
	ctx := context.Background()
	files := map[string][]byte{}
	if ios {
		t := *team
		if t == "" {
			if t, err = signingTeam(ctx, cfg); err != nil {
				return fmt.Errorf("cannot find the Apple team ID (%v); pass -team (developer.apple.com, Membership details), or -platform android", err)
			}
		}
		if !appleTeamPattern.MatchString(t) {
			return fmt.Errorf("team ID %q must be 10 upper-case letters or digits", t)
		}
		if files["apple-app-site-association"], err = appleAppSiteAssociation([]string{t + "." + cfg.ID}, cfg.AppLinkPaths); err != nil {
			return err
		}
	}
	if android {
		fingerprints, err := androidFingerprints(ctx, cfg, fps, *debug)
		if err != nil {
			return err
		}
		if files["assetlinks.json"], err = assetLinks(cfg.ID, fingerprints); err != nil {
			return err
		}
	}
	wellKnown := filepath.Join(*out, ".well-known")
	if !filepath.IsAbs(wellKnown) {
		wellKnown = cfg.Path(wellKnown)
	}
	if err := os.MkdirAll(wellKnown, 0o755); err != nil {
		return err
	}
	for _, name := range sortedKeys(files) {
		p := filepath.Join(wellKnown, name)
		if err := os.WriteFile(p, files[name], 0o644); err != nil {
			return err
		}
		logf("wrote %s", p)
	}
	printAppLinksHosting(os.Stdout, cfg, sortedKeys(files))
	return nil
}

// androidFingerprints are the -fingerprint values, else the release (or,
// with -debug, the debug) key's.
func androidFingerprints(ctx context.Context, cfg *Config, flags []string, debug bool) ([]string, error) {
	var out []string
	for _, f := range flags {
		fp, err := normalizeFingerprint(f)
		if err != nil {
			return nil, fmt.Errorf("-fingerprint: %w", err)
		}
		out = append(out, fp)
	}
	if len(out) > 0 {
		return out, nil
	}
	var fp string
	var err error
	if debug {
		ks := debugKeystore()
		if !fileExists(ks) {
			return nil, fmt.Errorf("%s does not exist; build a debug APK once (tan build -platform android -debug) to create it", ks)
		}
		warnf("assetlinks.json uses the debug key %s: it verifies debug builds only; publish the release (or Play App Signing) fingerprint for real users", ks)
		fp, err = keytoolFingerprint(ctx, ks, "androiddebugkey", "android")
	} else {
		if cfg.Android.Keystore == "" {
			return nil, errors.New(`assetlinks.json needs the signing certificate: set "android.keystore" and "android.keyAlias", pass -fingerprint (Play App Signing: Play Console → Test and release → App integrity), or -debug for debug builds`)
		}
		pass := os.Getenv(envAndroidStorePass)
		if pass == "" {
			return nil, fmt.Errorf("set $%s to the password of %s (or pass -fingerprint)", envAndroidStorePass, cfg.Android.Keystore)
		}
		fp, err = keytoolFingerprint(ctx, cfg.Path(cfg.Android.Keystore), cfg.Android.KeyAlias, pass)
		if err == nil {
			logf("assetlinks.json: fingerprint of %s (%s). With Play App Signing, add the app signing key with -fingerprint", cfg.Android.Keystore, cfg.Android.KeyAlias)
		}
	}
	if err != nil {
		return nil, err
	}
	return []string{fp}, nil
}

// printAppLinksHosting says where the files go.
func printAppLinksHosting(w io.Writer, cfg *Config, files []string) {
	fmt.Fprintln(w, "Serve these files over https (no redirects; Content-Type: application/json):")
	var hosts []string
	for _, d := range cfg.AppLinks {
		if h := appLinkHost(d); !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	for _, h := range hosts {
		for _, f := range files {
			fmt.Fprintf(w, "  https://%s/.well-known/%s\n", h, f)
		}
	}
}

// appLinksCheck is tan doctor's row for "appLinks": a warning when the
// iOS provisioning profile cannot sign the associated-domains entitlement.
func appLinksCheck(dir string) []check {
	if _, err := os.Stat(filepath.Join(dir, ConfigFile)); err != nil {
		return nil
	}
	cfg, err := LoadConfig(dir)
	if err != nil || len(cfg.AppLinks) == 0 {
		return nil // a broken tan.json is reported by the icon checks
	}
	c := check{name: "App Links", status: statusOK, detail: strings.Join(cfg.AppLinks, ", ")}
	if runtime.GOOS == "darwin" && cfg.IOS.ProvisioningProfile != "" {
		p, err := readProfile(context.Background(), cfg.Path(cfg.IOS.ProvisioningProfile))
		switch {
		case err != nil:
			c.status, c.detail = statusWarn, err.Error()
		case !profileHasAssociatedDomains(p):
			c.status, c.detail = statusWarn, fmt.Sprintf("%s does not grant Associated Domains: enable it for %s at developer.apple.com and download the profile again", cfg.IOS.ProvisioningProfile, cfg.ID)
		}
	}
	return []check{c}
}

// plistRaw is a <date> or <data> value kept as is.
type plistRaw struct{ tag, text string }

// decodePlist parses an XML property list whose root is a dictionary.
func decodePlist(data []byte) (plist, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false // the plist DOCTYPE
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("not an XML property list: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local != "plist" {
			v, err := decodePlistValue(dec, se)
			if err != nil {
				return nil, err
			}
			p, ok := v.(plist)
			if !ok {
				return nil, errors.New("property list root is not a dictionary")
			}
			return p, nil
		}
	}
}

func decodePlistValue(dec *xml.Decoder, se xml.StartElement) (any, error) {
	switch se.Name.Local {
	case "dict":
		p := plist{}
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.EndElement:
				return p, nil
			case xml.StartElement:
				if t.Name.Local != "key" {
					return nil, fmt.Errorf("plist: <%s> where a <key> belongs", t.Name.Local)
				}
				var key string
				if err := dec.DecodeElement(&key, &t); err != nil {
					return nil, err
				}
				vt, err := nextStart(dec)
				if err != nil {
					return nil, err
				}
				v, err := decodePlistValue(dec, vt)
				if err != nil {
					return nil, err
				}
				p = append(p, plistEntry{key, v})
			}
		}
	case "array":
		vs := []any{}
		for {
			tok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			switch t := tok.(type) {
			case xml.EndElement:
				return vs, nil
			case xml.StartElement:
				v, err := decodePlistValue(dec, t)
				if err != nil {
					return nil, err
				}
				vs = append(vs, v)
			}
		}
	case "true", "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return se.Name.Local == "true", nil
	}
	var text string
	if err := dec.DecodeElement(&text, &se); err != nil {
		return nil, err
	}
	switch se.Name.Local {
	case "string":
		return text, nil
	case "integer":
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("plist: bad <integer> %q", text)
		}
		return n, nil
	case "real":
		f, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return nil, fmt.Errorf("plist: bad <real> %q", text)
		}
		return f, nil
	case "date", "data":
		return plistRaw{se.Name.Local, strings.TrimSpace(text)}, nil
	}
	return nil, fmt.Errorf("plist: unknown element <%s>", se.Name.Local)
}

func nextStart(dec *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := dec.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return t, nil
		case xml.EndElement:
			return xml.StartElement{}, errors.New("plist: <key> without a value")
		}
	}
}
