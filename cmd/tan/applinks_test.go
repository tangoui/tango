package main

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const appLinksJSON = `{"name":"Hello","id":"com.example.hello","version":"1.0.0",
	"frontend":{"dir":"web","dist":"."},
	"urlSchemes":["hello"],
	"appLinks":["example.com","*.example.org"],
	"appLinkPaths":["/open/","/share"]}`

func appLinksConfig(t *testing.T) *Config {
	t.Helper()
	c, err := ParseConfig([]byte(appLinksJSON))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestAppLinksConfig(t *testing.T) {
	c := appLinksConfig(t)
	if want := []string{"applinks:example.com", "applinks:*.example.org"}; !reflect.DeepEqual(c.associatedDomains(), want) {
		t.Errorf("associatedDomains = %v, want %v", c.associatedDomains(), want)
	}
	base := `{"name":"A","id":"com.example.a","version":"1.0.0","frontend":{"dir":"w","dist":"."},%s}`
	for _, bad := range []struct{ field, msg string }{
		{`"appLinks":["https://example.com"]`, "without a scheme"},
		{`"appLinks":["example.com/app"]`, "without a path"},
		{`"appLinks":["example.com:8443"]`, "port"},
		{`"appLinks":["Example.com"]`, "lower case"},
		{`"appLinks":["localhost"]`, "not a domain"},
		{`"appLinks":["*.*.example.com"]`, "not a domain"},
		{`"appLinks":["exa_mple.com"]`, "not a domain"},
		{`"appLinks":["-a.example.com"]`, "not a domain"},
		{`"appLinks":["example.com","example.com"]`, "listed twice"},
		{`"appLinkPaths":["/open"]`, `needs "appLinks"`},
		{`"appLinks":["example.com"],"appLinkPaths":["open"]`, "starting with"},
		{`"appLinks":["example.com"],"appLinkPaths":["/open/*"]`, "starting with"},
		{`"appLinks":["example.com"],"appLinkPaths":["/a?b"]`, "starting with"},
	} {
		_, err := ParseConfig([]byte(strings.Replace(base, "%s", bad.field, 1)))
		if err == nil || !strings.Contains(err.Error(), bad.msg) {
			t.Errorf("%s: error %v, want one containing %q", bad.field, err, bad.msg)
		}
	}
	for _, good := range []string{`"appLinks":["a.b"]`, `"appLinks":["*.example.com","example.com","xn--fiqs8s.cn"]`, `"appLinks":["example.com"],"appLinkPaths":["/","/a-b/c_d.e~"]`} {
		if _, err := ParseConfig([]byte(strings.Replace(base, "%s", good, 1))); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
}

func TestWithAssociatedDomains(t *testing.T) {
	domains := []string{"applinks:example.com", "applinks:*.example.org"}
	// A provisioning profile grants the capability as "*": replaced.
	profile := plist{
		{"application-identifier", "ABCDE12345.com.example.hello"},
		{associatedDomainsKey, "*"},
		{"get-task-allow", true},
	}
	got := withAssociatedDomains(profile, domains)
	want := plist{
		{"application-identifier", "ABCDE12345.com.example.hello"},
		{associatedDomainsKey, []any{"applinks:example.com", "applinks:*.example.org"}},
		{"get-task-allow", true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("profile merge = %#v\nwant %#v", got, want)
	}
	if profile.get(associatedDomainsKey) != "*" {
		t.Error("withAssociatedDomains changed its input")
	}
	// Existing entries (from ios.entitlements) are kept, duplicates dropped.
	own := plist{{associatedDomainsKey, []any{"webcredentials:example.com", "applinks:example.com"}}}
	got = withAssociatedDomains(own, domains)
	if want := []any{"webcredentials:example.com", "applinks:example.com", "applinks:*.example.org"}; !reflect.DeepEqual(got.get(associatedDomainsKey), want) {
		t.Errorf("own merge = %v, want %v", got.get(associatedDomainsKey), want)
	}
	// No entitlements at all: just the key.
	got = withAssociatedDomains(nil, domains)
	if len(got) != 1 || got[0].key != associatedDomainsKey {
		t.Errorf("empty merge = %#v", got)
	}
}

func TestEntitlementsFileRoundTrip(t *testing.T) {
	src := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>application-identifier</key>
	<string>ABCDE12345.com.example.hello</string>
	<key>com.apple.developer.associated-domains</key>
	<string>*</string>
	<key>com.apple.developer.team-identifier</key>
	<string>ABCDE12345</string>
	<key>get-task-allow</key>
	<false/>
	<key>keychain-access-groups</key>
	<array>
		<string>ABCDE12345.*</string>
	</array>
	<key>n</key>
	<integer>3</integer>
	<key>blob</key>
	<data>
	AAEC
	</data>
	<key>when</key>
	<date>2026-01-01T00:00:00Z</date>
	<key>nested</key>
	<dict/>
</dict>
</plist>
`
	dir := t.TempDir()
	base := filepath.Join(dir, "profile.entitlements")
	if err := os.WriteFile(base, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out.plist")
	if err := writeAppLinkEntitlements(t.Context(), appLinksConfig(t), base, out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(out)
	if err := xml.Unmarshal(data, new(any)); err != nil {
		t.Fatalf("output is not XML: %v\n%s", err, data)
	}
	got, err := decodePlist(data)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.get(associatedDomainsKey); !reflect.DeepEqual(v, []any{"applinks:example.com", "applinks:*.example.org"}) {
		t.Errorf("associated domains = %#v", v)
	}
	for key, want := range map[string]any{
		"application-identifier": "ABCDE12345.com.example.hello",
		"get-task-allow":         false,
		"keychain-access-groups": []any{"ABCDE12345.*"},
		"n":                      3,
		"blob":                   plistRaw{"data", "AAEC"},
		"when":                   plistRaw{"date", "2026-01-01T00:00:00Z"},
		"nested":                 plist{},
	} {
		if v := got.get(key); !reflect.DeepEqual(v, want) {
			t.Errorf("%s = %#v, want %#v", key, v, want)
		}
	}
	// Profiles: capability and team.
	prof := plist{{"TeamIdentifier", []any{"ABCDE12345"}}, {"Entitlements", got}}
	if !profileHasAssociatedDomains(prof) || profileTeam(prof) != "ABCDE12345" {
		t.Error("profile with associated domains not recognized")
	}
	if profileHasAssociatedDomains(plist{{"Entitlements", plist{{"get-task-allow", true}}}}) {
		t.Error("profile without associated domains accepted")
	}
}

func TestSimulatorEntitlementsAppLinks(t *testing.T) {
	ent := simulatorEntitlements(appLinksConfig(t))
	if v := ent.get(associatedDomainsKey); !reflect.DeepEqual(v, []any{"applinks:example.com", "applinks:*.example.org"}) {
		t.Errorf("simulator associated domains = %#v", v)
	}
	plain := mobileConfig(t)
	if simulatorEntitlements(plain).get(associatedDomainsKey) != nil {
		t.Error("associated domains without appLinks")
	}
}

func TestAndroidManifestAppLinks(t *testing.T) {
	c := appLinksConfig(t)
	got, err := androidManifest(manifestTemplate, c, false)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(got, `<intent-filter android:autoVerify="true">`); n != 2 {
		t.Errorf("%d autoVerify filters, want 2:\n%s", n, got)
	}
	want := `            <intent-filter android:autoVerify="true">
                <action android:name="android.intent.action.VIEW" />
                <category android:name="android.intent.category.DEFAULT" />
                <category android:name="android.intent.category.BROWSABLE" />
                <data android:scheme="https" />
                <data android:scheme="http" />
                <data android:host="*.example.org" />
                <data android:pathPrefix="/open/" />
                <data android:pathPrefix="/share" />
            </intent-filter>
        </activity>`
	if !strings.Contains(got, want) {
		t.Errorf("manifest lacks\n%s\n---\n%s", want, got)
	}
	// After the deep-link filter, inside the host Activity.
	scheme := strings.Index(got, `<data android:scheme="hello" />`)
	host := strings.Index(got, `android:host="example.com"`)
	if scheme < 0 || host < scheme || host > strings.Index(got, "</activity>") {
		t.Errorf("app link filter misplaced:\n%s", got)
	}
	if !strings.Contains(got, "https://example.org/.well-known/assetlinks.json") {
		t.Error("wildcard comment should name the root domain")
	}
	if err := xml.Unmarshal([]byte(got), new(any)); err != nil {
		t.Errorf("manifest is not XML: %v", err)
	}
	// Idempotent: a second pass adds nothing.
	again, err := androidAppLinkFilters(got, c)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Errorf("second pass changed the manifest:\n%s", again)
	}
	// Without appLinks nothing changes.
	plain := mobileConfig(t)
	if same, _ := androidAppLinkFilters(manifestTemplate, plain); same != manifestTemplate {
		t.Error("filters added without appLinks")
	}
}

func TestAppleAppSiteAssociation(t *testing.T) {
	b, err := appleAppSiteAssociation([]string{"ABCDE12345.com.example.hello"}, []string{"/open/", "/share"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Applinks struct {
			Details []struct {
				AppIDs     []string            `json:"appIDs"`
				Components []map[string]string `json:"components"`
			} `json:"details"`
		} `json:"applinks"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	d := doc.Applinks.Details
	if len(d) != 1 || !reflect.DeepEqual(d[0].AppIDs, []string{"ABCDE12345.com.example.hello"}) {
		t.Fatalf("details = %+v", d)
	}
	if want := []map[string]string{{"/": "/open/*"}, {"/": "/share*"}}; !reflect.DeepEqual(d[0].Components, want) {
		t.Errorf("components = %v, want %v", d[0].Components, want)
	}
	all, _ := appleAppSiteAssociation([]string{"T.x.y"}, nil)
	if !strings.Contains(string(all), `"/": "*"`) {
		t.Errorf("no paths should match every path:\n%s", all)
	}
}

func TestAssetLinks(t *testing.T) {
	fp, err := normalizeFingerprint(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.TrimSuffix(strings.Repeat("AB:", 32), ":"); fp != want {
		t.Errorf("normalized %q", fp)
	}
	for _, bad := range []string{"", "AB:CD", strings.Repeat("ZZ", 32), strings.Repeat("AB", 20)} {
		if _, err := normalizeFingerprint(bad); err == nil {
			t.Errorf("fingerprint %q accepted", bad)
		}
	}
	b, err := assetLinks("com.example.hello", []string{fp})
	if err != nil {
		t.Fatal(err)
	}
	var doc []struct {
		Relation []string `json:"relation"`
		Target   struct {
			Namespace    string   `json:"namespace"`
			PackageName  string   `json:"package_name"`
			Fingerprints []string `json:"sha256_cert_fingerprints"`
		} `json:"target"`
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if len(doc) != 1 || !reflect.DeepEqual(doc[0].Relation, []string{"delegate_permission/common.handle_all_urls"}) ||
		doc[0].Target.Namespace != "android_app" || doc[0].Target.PackageName != "com.example.hello" ||
		!reflect.DeepEqual(doc[0].Target.Fingerprints, []string{fp}) {
		t.Errorf("assetlinks.json = %s", b)
	}
}

func TestKeytoolSHA256(t *testing.T) {
	out := "Alias name: upload\nCertificate fingerprints:\n\t SHA1: 00:11\n\t SHA256: AB:CD:EF\nSignature algorithm name: SHA256withRSA\n"
	if got := keytoolSHA256(out); got != "AB:CD:EF" {
		t.Errorf("keytoolSHA256 = %q", got)
	}
	if keytoolSHA256("nothing") != "" {
		t.Error("found a fingerprint in nothing")
	}
}

func TestAppLinkHosting(t *testing.T) {
	var b strings.Builder
	c := appLinksConfig(t)
	c.AppLinks = append(c.AppLinks, "example.org")
	printAppLinksHosting(&b, c, []string{"assetlinks.json"})
	got := b.String()
	for _, want := range []string{"https://example.com/.well-known/assetlinks.json", "https://example.org/.well-known/assetlinks.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("hosting lacks %s:\n%s", want, got)
		}
	}
	if strings.Count(got, "example.org/") != 1 {
		t.Errorf("example.org listed twice:\n%s", got)
	}
}
