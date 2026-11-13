package main

import (
	"archive/zip"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const mobileJSON = `{
  "name": "Hello",
  "id": "com.example.hello",
  "version": "1.2.3-beta.1",
  "frontend": {"dir": "web", "dist": "."},
  "urlSchemes": ["hello"],
  "permissions": ["camera", "photos", "location"],
  "ios": {
    "minVersion": "16.4",
    "build": "42",
    "signIdentity": "Apple Development: Jane Doe (ABCDE12345)",
    "provisioningProfile": "ios/dev.mobileprovision",
    "usageDescriptions": {"camera": "Scan <codes> & more"}
  },
  "android": {"minSdk": 26, "targetSdk": 36, "keystore": "release.jks", "keyAlias": "upload"}
}`

func TestParseConfigMobile(t *testing.T) {
	c, err := ParseConfig([]byte(mobileJSON))
	if err != nil {
		t.Fatal(err)
	}
	if c.IOSMinVersion() != "16.4" || c.IOSShortVersion() != "1.2.3" || c.IOSBuild() != "42" {
		t.Errorf("ios: min %s short %s build %s", c.IOSMinVersion(), c.IOSShortVersion(), c.IOSBuild())
	}
	if c.AndroidMinSdk() != 26 || c.AndroidTargetSdk() != 36 || c.AndroidVersionCode() != 10203 {
		t.Errorf("android: %d %d %d", c.AndroidMinSdk(), c.AndroidTargetSdk(), c.AndroidVersionCode())
	}

	base := `"name":"A","id":"a.b","version":"1","frontend":{"dir":"f","dist":"."}`
	bad := []struct{ json, want string }{
		{`"permissions":["camra"]`, `unknown permission "camra"`},
		{`"permissions":["camera","camera"]`, `listed twice`},
		{`"permissions":["biometrics"]`, `unknown permission "biometrics"`},
		{`"ios":{"minVersion":"seventeen"}`, `"ios.minVersion"`},
		{`"ios":{"build":"1a"}`, `"ios.build"`},
		{`"ios":{"usageDescriptions":{"camera":"x"}}`, `not in "permissions"`},
		{`"permissions":["notifications"],"ios":{"usageDescriptions":{"notifications":"x"}}`, `no usage description`},
		{`"permissions":["camera"],"ios":{"usageDescriptions":{"camera":" "}}`, `is empty`},
		{`"ios":{"provisioningProfile":"/abs/p.mobileprovision"}`, `relative`},
		{`"ios":{"signingIdentity":"x"}`, `unknown field "signingIdentity"`},
		{`"android":{"minSdk":21}`, `"android.minSdk" 21`},
		{`"android":{"minSdk":30,"targetSdk":29}`, `"android.targetSdk" 29`},
		{`"android":{"keystore":"k.jks"}`, `go together`},
		{`"android":{"versionCode":-1}`, `"android.versionCode"`},
	}
	for _, b := range bad {
		js := "{" + base + "," + b.json + "}"
		_, err := ParseConfig([]byte(js))
		if err == nil || !strings.Contains(err.Error(), b.want) {
			t.Errorf("%s: err = %v, want it to mention %s", b.json, err, b.want)
		}
	}
}

func TestAndroidVersionCode(t *testing.T) {
	for v, want := range map[string]int{"0.1.0": 100, "1.2.3": 10203, "2": 20000, "1.150.7": 19907, "0.0.0": 1, "3.1.4+git": 30104} {
		c := &Config{Version: v}
		if got := c.AndroidVersionCode(); got != want {
			t.Errorf("%s: %d, want %d", v, got, want)
		}
	}
	if got := (&Config{Version: "1.0.0", Android: Android{VersionCode: 7}}).AndroidVersionCode(); got != 7 {
		t.Errorf("explicit versionCode: %d", got)
	}
}

func TestParseTargetsMobile(t *testing.T) {
	cases := map[string][]target{
		"android":                           {{"android", "arm64"}, {"android", "amd64"}},
		"android/arm64,android/amd64":       {{"android", "arm64"}, {"android", "amd64"}},
		"android/arm64-v8a,android/x86":     {{"android", "arm64"}, {"android", "386"}},
		"ios":                               {{"ios", "arm64"}},
		"ios-simulator/arm64, darwin/arm64": {{"ios-simulator", "arm64"}, {"darwin", "arm64"}},
		"android/armeabi-v7a,android/arm":   {{"android", "arm"}},
	}
	for in, want := range cases {
		got, err := parseTargets(in)
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("%s: %v, %v; want %v", in, got, err, want)
		}
	}
	desktop, m := splitTargets([]target{{"android", "arm64"}, {"darwin", "arm64"}, {"ios", "arm64"}, {"android", "amd64"}})
	if !slices.Equal(desktop, []target{{"darwin", "arm64"}}) || !slices.Equal(m.androidArches, []string{"arm64", "amd64"}) || len(m.ios) != 1 {
		t.Errorf("split: %v %+v", desktop, m)
	}
	if androidABI("arm") != "armeabi-v7a" || androidABI("amd64") != "x86_64" {
		t.Error("androidABI")
	}
}

func mobileConfig(t *testing.T) *Config {
	t.Helper()
	c, err := ParseConfig([]byte(mobileJSON))
	if err != nil {
		t.Fatal(err)
	}
	c.dir = t.TempDir()
	return c
}

func lintPlist(t *testing.T, name string, data []byte) {
	t.Helper()
	plutil, err := exec.LookPath("plutil")
	if err != nil {
		return
	}
	p := filepath.Join(t.TempDir(), name)
	os.WriteFile(p, data, 0o644)
	if out, err := exec.Command(plutil, "-lint", p).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %v: %s\n%s", err, out, data)
	}
}

func TestIOSInfoPlist(t *testing.T) {
	c := mobileConfig(t)
	icons := map[string]any{"CFBundleIcons": map[string]any{"CFBundlePrimaryIcon": map[string]any{
		"CFBundleIconName": "AppIcon", "CFBundleIconFiles": []any{"AppIcon60x60"}}}}
	got := string(encodePlist(iosInfoPlist(c, iosSDK{platform: "iPhoneSimulator"}, icons)))
	for _, want := range []string{
		"<key>CFBundleExecutable</key>\n\t<string>Hello</string>",
		"<key>CFBundleShortVersionString</key>\n\t<string>1.2.3</string>",
		"<key>CFBundleVersion</key>\n\t<string>42</string>",
		"<key>MinimumOSVersion</key>\n\t<string>16.4</string>",
		"<key>CFBundleSupportedPlatforms</key>\n\t<array>\n\t\t<string>iPhoneSimulator</string>",
		"<key>UISceneDelegateClassName</key>\n\t\t\t\t\t<string>TangoSceneDelegate</string>",
		"<key>UILaunchScreen</key>\n\t<dict/>",
		"<key>CADisableMinimumFrameDurationOnPhone</key>\n\t<true/>",
		"<key>CFBundleURLSchemes</key>\n\t\t\t<array>\n\t\t\t\t<string>hello</string>",
		"<key>NSCameraUsageDescription</key>\n\t<string>Scan &lt;codes&gt; &amp; more</string>",
		"<key>NSPhotoLibraryUsageDescription</key>\n\t<string>Hello uses your photo library.</string>",
		"<key>NSPhotoLibraryAddUsageDescription</key>",
		"<key>NSLocationWhenInUseUsageDescription</key>\n\t<string>Hello uses your location.</string>",
		"<key>CFBundleIconName</key>\n\t\t\t<string>AppIcon</string>",
		"<key>UIDeviceFamily</key>\n\t<array>\n\t\t<integer>1</integer>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "NSFaceIDUsageDescription") {
		t.Error("undeclared permission in Info.plist")
	}
	lintPlist(t, "Info.plist", []byte(got))
}

const manifestTemplate = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <uses-permission android:name="android.permission.INTERNET" />
    <application
        android:label="@string/app_name"
        android:icon="@android:drawable/sym_def_app_icon">
        <activity android:name="org.tango.host.Other" />
        <activity
            android:name="org.tango.host.TangoActivity"
            android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
`

func TestAndroidManifest(t *testing.T) {
	c := mobileConfig(t)
	got, err := androidManifest(manifestTemplate, c, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<uses-permission android:name="android.permission.CAMERA" />`,
		`<uses-permission android:name="android.permission.READ_MEDIA_IMAGES" />`,
		`<uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" android:maxSdkVersion="32" />`,
		`<uses-permission android:name="android.permission.ACCESS_FINE_LOCATION" />`,
		`<uses-feature android:name="android.hardware.camera" android:required="false" />`,
		`android:icon="@mipmap/ic_launcher"`,
		"\n        android:roundIcon=\"@mipmap/ic_launcher_round\">",
		`<data android:scheme="hello" />`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("manifest lacks %q:\n%s", want, got)
		}
	}
	if strings.Count(got, "android.permission.INTERNET") != 1 {
		t.Error("INTERNET declared twice")
	}
	// Permissions go before <application>; the deep link into the host
	// Activity, not the first one.
	if strings.Index(got, "CAMERA") > strings.Index(got, "<application") {
		t.Error("permissions after <application>")
	}
	host := strings.Index(got, `"org.tango.host.TangoActivity"`)
	if d := strings.Index(got, "android.intent.action.VIEW"); d < host || d > strings.LastIndex(got, "</activity>") {
		t.Errorf("deep link not in the host activity:\n%s", got)
	}

	plain := &Config{Name: "A", ID: "a.b", Version: "1"}
	same, err := androidManifest(manifestTemplate, plain, false)
	if err != nil || same != manifestTemplate {
		t.Errorf("no settings changed the manifest: %v\n%s", err, same)
	}
	if _, err := androidManifest("<manifest/>", plain, false); err == nil {
		t.Error("template without <application> accepted")
	}
	plain.URLSchemes = []string{"x"}
	if _, err := androidManifest(strings.ReplaceAll(manifestTemplate, "TangoActivity", "Main"), plain, false); err == nil {
		t.Error("template without the host activity accepted")
	}
}

func TestAndroidString(t *testing.T) {
	for in, want := range map[string]string{
		`Bob's <App>`: `Bob\&#39;s &lt;App&gt;`,
		`@home`:       `\@home`,
		`Say "hi"`:    `Say \&#34;hi\&#34;`,
	} {
		if got := androidString(in); got != want {
			t.Errorf("androidString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGradleEnv(t *testing.T) {
	c := mobileConfig(t)
	env := gradleEnv(c, androidRelease{keystore: `C:\R&D\app\release.jks`, alias: "upload"})
	got := strings.Join(env, "\n")
	for _, want := range []string{"ORG_GRADLE_PROJECT_tangoAppId=com.example.hello", "ORG_GRADLE_PROJECT_tangoVersionCode=10203", "ORG_GRADLE_PROJECT_tangoMinSdk=26",
		`ORG_GRADLE_PROJECT_tangoKeystore=C:\R&D\app\release.jks`, "ORG_GRADLE_PROJECT_tangoKeyAlias=upload"} {
		if !slices.Contains(env, want) {
			t.Errorf("env lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(strings.Join(gradleEnv(c, androidRelease{}), " "), "Keystore") {
		t.Error("keystore passed for a debug build")
	}
}

func TestGradleCommandWindows(t *testing.T) {
	proj := filepath.Join(t.TempDir(), "project")
	args := []string{"--offline", "assembleRelease", "bundleRelease"}
	cmd, err := gradleCommand(t.Context(), "windows", proj, args)
	if err != nil {
		t.Fatal(err)
	}
	// gradlew.bat itself, not cmd /c: nothing from tan.json on the line.
	if cmd.Path != filepath.Join(proj, "gradlew.bat") || cmd.Args[0] != "gradlew.bat" || cmd.Dir != proj {
		t.Errorf("Path %q Args %q Dir %q", cmd.Path, cmd.Args, cmd.Dir)
	}
	if !slices.Equal(cmd.Args[1:], args) {
		t.Errorf("args %q", cmd.Args)
	}
	for _, a := range cmd.Args {
		if strings.EqualFold(filepath.Base(a), "cmd") || strings.EqualFold(a, "cmd.exe") || a == "/c" || strings.HasPrefix(a, "-P") {
			t.Errorf("command line has %q: %q", a, cmd.Args)
		}
	}
	for _, bad := range []string{"-Pa=1&calc", "a|b", "x^y", "%PATH%", `"q"`, "(x)"} {
		if _, err := gradleCommand(t.Context(), "windows", proj, []string{bad}); err == nil {
			t.Errorf("argument %q accepted", bad)
		}
	}
	if _, err := gradleCommand(t.Context(), "windows", `C:\R&D\app\build\android\project`, args); err == nil || !strings.Contains(err.Error(), "&") {
		t.Errorf("project path with &: %v", err)
	}
	if _, err := gradleCommand(t.Context(), "windows", `C:\Program Files (x86)\app\project`, args); err != nil {
		t.Errorf("path with spaces and parentheses: %v", err)
	}

	cmd, err = gradleCommand(t.Context(), "darwin", proj, args)
	if err != nil || cmd.Args[0] != "sh" || cmd.Args[1] != "gradlew" || cmd.Dir != proj {
		t.Errorf("unix: %q %v", cmd.Args, err)
	}
}

func TestRunGradleEnv(t *testing.T) {
	var gotCmd *exec.Cmd
	var gotEnv []string
	old := execGradle
	execGradle = func(cmd *exec.Cmd, env []string) error { gotCmd, gotEnv = cmd, env; return nil }
	t.Cleanup(func() { execGradle = old })
	c := mobileConfig(t)
	c.Android.KeyAlias = "upload.key_1-a"
	rel := androidRelease{keystore: c.Path("release.jks"), alias: c.Android.KeyAlias}
	if err := runGradle(t.Context(), t.TempDir(), []string{"assembleRelease"}, gradleEnv(c, rel)); err != nil {
		t.Fatal(err)
	}
	line := strings.Join(gotCmd.Args, " ")
	if strings.Contains(line, rel.keystore) || strings.Contains(line, rel.alias) || strings.Contains(line, "tangoAppId") {
		t.Errorf("identity or key on the command line: %s", line)
	}
	if !slices.Contains(gotEnv, "ORG_GRADLE_PROJECT_tangoKeyAlias=upload.key_1-a") || !slices.Contains(gotEnv, "ORG_GRADLE_PROJECT_tangoKeystore="+rel.keystore) {
		t.Errorf("env %q", gotEnv)
	}
}

func TestKeyAliasValidation(t *testing.T) {
	for alias, ok := range map[string]bool{"upload": true, "my.key_1-a": true, "upload&calc": false, "a b": false, `a"b`: false, "a|b": false, "%x%": false, "a\nb": false} {
		c := mobileConfig(t)
		c.Android.KeyAlias = alias
		err := c.validate()
		if ok != (err == nil) {
			t.Errorf("validate keyAlias %q: %v", alias, err)
		}
		os.WriteFile(c.Path("release.jks"), []byte("x"), 0o644)
		t.Setenv(envAndroidStorePass, "pw")
		if _, err := androidReleaseConfig(c); ok != (err == nil) {
			t.Errorf("androidReleaseConfig keyAlias %q: %v", alias, err)
		}
	}
}

func TestAndroidReleaseConfig(t *testing.T) {
	c := mobileConfig(t)
	if _, err := androidReleaseConfig(c); err == nil || !strings.Contains(err.Error(), "release.jks") {
		t.Errorf("missing keystore file: %v", err)
	}
	os.WriteFile(c.Path("release.jks"), []byte("x"), 0o644)
	t.Setenv(envAndroidStorePass, "")
	if _, err := androidReleaseConfig(c); err == nil || !strings.Contains(err.Error(), envAndroidStorePass) {
		t.Errorf("missing password: %v", err)
	}
	t.Setenv(envAndroidStorePass, "pw")
	if r, err := androidReleaseConfig(c); err != nil || r.alias != "upload" {
		t.Errorf("%+v %v", r, err)
	}
	c.Android = Android{}
	if _, err := androidReleaseConfig(c); err == nil || !strings.Contains(err.Error(), "-debug") {
		t.Errorf("no keystore: %v", err)
	}
}

func TestIOSSigningConfig(t *testing.T) {
	c := mobileConfig(t)
	t.Setenv(envIOSSignIdentity, "")
	if _, err := iosSigningConfig(c); err == nil || !strings.Contains(err.Error(), "dev.mobileprovision") {
		t.Errorf("missing profile file: %v", err)
	}
	c.IOS = IOS{}
	_, err := iosSigningConfig(c)
	if err == nil || !strings.Contains(err.Error(), `"ios.signIdentity"`) || !strings.Contains(err.Error(), `"ios.provisioningProfile"`) || !strings.Contains(err.Error(), "ios-simulator") {
		t.Errorf("unsigned: %v", err)
	}
	os.WriteFile(c.Path("p.mobileprovision"), []byte("x"), 0o644)
	c.IOS.ProvisioningProfile = "p.mobileprovision"
	t.Setenv(envIOSSignIdentity, "Apple Development: X (ABCDE12345)")
	if s, err := iosSigningConfig(c); err != nil || s.identity == "" || s.entitlements != "" {
		t.Errorf("%+v %v", s, err)
	}
}

func TestProfileCovers(t *testing.T) {
	for _, c := range []struct {
		appID, bundle string
		want          bool
	}{
		{"ABCDE12345.com.example.hello", "com.example.hello", true},
		{"ABCDE12345.com.example.hello", "com.example.hello2", false},
		{"ABCDE12345.*", "com.example.hello", true},
		{"ABCDE12345.com.example.*", "com.example.hello", true},
		{"ABCDE12345.org.*", "com.example.hello", false},
		{"bogus", "com.example.hello", false},
	} {
		if got := profileCovers(c.appID, c.bundle); got != c.want {
			t.Errorf("profileCovers(%q, %q) = %v", c.appID, c.bundle, got)
		}
	}
}

func TestDedupeLinkFlags(t *testing.T) {
	got := dedupeLinkFlags([]string{"-framework", "UIKit", "", "-lresolv", "-framework", "AVFoundation", "-framework", "UIKit", "-lresolv", "-weak_framework", "UIKit"})
	want := []string{"-framework", "UIKit", "-lresolv", "-framework", "AVFoundation", "-weak_framework", "UIKit"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestWriteIPA(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "Hello.app")
	os.MkdirAll(filepath.Join(app, "_CodeSignature"), 0o755)
	os.WriteFile(filepath.Join(app, "Hello"), []byte("bin"), 0o755)
	os.WriteFile(filepath.Join(app, "Info.plist"), []byte("plist"), 0o644)
	// A Mach-O library without execute bits, as every file is on Windows.
	os.MkdirAll(filepath.Join(app, "Frameworks"), 0o755)
	os.WriteFile(filepath.Join(app, "Frameworks", "libx.dylib"), []byte("\xcf\xfa\xed\xfe rest"), 0o644)
	ipa := filepath.Join(dir, "Hello.ipa")
	if err := writeIPA(ipa, app); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(ipa)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	modes := map[string]os.FileMode{}
	for _, f := range zr.File {
		modes[f.Name] = f.Mode()
	}
	if m, ok := modes["Payload/Hello.app/Hello"]; !ok || m&0o111 == 0 {
		t.Errorf("executable missing or not executable: %v", modes)
	}
	if m := modes["Payload/Hello.app/Frameworks/libx.dylib"]; m&0o111 == 0 {
		t.Errorf("Mach-O library not executable: %v", m)
	}
	if m := modes["Payload/Hello.app/Info.plist"]; m&0o111 != 0 {
		t.Errorf("Info.plist executable: %v", m)
	}
	if _, ok := modes["Payload/Hello.app/_CodeSignature/"]; !ok {
		t.Errorf("directories missing: %v", modes)
	}
}

func TestResizeIcon(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			if x >= 50 {
				src.Set(x, y, color.NRGBA{255, 0, 0, 255})
			}
		}
	}
	got := resizeIcon(src, 10, nil)
	if got.Bounds().Dx() != 10 || got.NRGBAAt(9, 5).R != 255 || got.NRGBAAt(0, 5).A != 0 {
		t.Errorf("transparent resize: %v %v", got.NRGBAAt(9, 5), got.NRGBAAt(0, 5))
	}
	flat := resizeIcon(src, 10, color.White)
	if c := flat.NRGBAAt(0, 5); c != (color.NRGBA{255, 255, 255, 255}) {
		t.Errorf("flattened transparent pixel = %v", c)
	}
}

func TestADBParsing(t *testing.T) {
	ds := parseADBDevices("* daemon started successfully\nList of devices attached\nemulator-5554\tdevice\nR58N\tunauthorized\n\n")
	if len(ds) != 2 || ds[0] != (adbDevice{"emulator-5554", "device"}) || ds[1].state != "unauthorized" {
		t.Errorf("devices %v", ds)
	}
	if a, err := pickABI("arm64-v8a,armeabi-v7a,armeabi\n"); err != nil || a != "arm64" {
		t.Errorf("pickABI = %s, %v", a, err)
	}
	if a, _ := pickABI("x86_64,arm64-v8a"); a != "amd64" {
		t.Errorf("pickABI x86_64 = %s", a)
	}
	if _, err := pickABI("mips"); err == nil {
		t.Error("mips accepted")
	}
}

func TestPickSimulator(t *testing.T) {
	ds := []simDevice{
		{UDID: "1", Name: "iPhone 16", State: "Shutdown", runtime: "com.apple.CoreSimulator.SimRuntime.iOS-18-6"},
		{UDID: "2", Name: "iPad Air", State: "Shutdown", runtime: "com.apple.CoreSimulator.SimRuntime.iOS-26-1"},
		{UDID: "3", Name: "iPhone 17", State: "Shutdown", runtime: "com.apple.CoreSimulator.SimRuntime.iOS-26-1"},
		{UDID: "4", Name: "iPhone 17 Pro", State: "Shutdown", runtime: "com.apple.CoreSimulator.SimRuntime.iOS-26-1"},
		{UDID: "5", Name: "iPhone Duo", State: "Shutdown", runtime: "com.apple.CoreSimulator.SimRuntime.iOS-26-1-iPhone19-4"},
	}
	if d, _ := pickSimulator(ds, ""); d.UDID != "4" {
		t.Errorf("default = %+v", d)
	}
	if d, _ := pickSimulator(ds, "iPhone 16"); d.UDID != "1" {
		t.Errorf("by name = %+v", d)
	}
	ds[1].State = "Booted"
	if d, _ := pickSimulator(ds, ""); d.UDID != "2" {
		t.Errorf("booted = %+v", d)
	}
	if _, err := pickSimulator(ds, "nope"); err == nil {
		t.Error("unknown device accepted")
	}
}

func TestPbxproj(t *testing.T) {
	c := mobileConfig(t)
	got := pbxproj(c, xcodeInputs{goBin: "/usr/local/go/bin/go", goDir: "..", extra: []string{"-framework", "AVFoundation"}, hasIcon: true, hasPrivacy: true, team: teamID(c.IOS.SignIdentity)})
	for _, want := range []string{
		"PRODUCT_BUNDLE_IDENTIFIER = com.example.hello;",
		"IPHONEOS_DEPLOYMENT_TARGET = 16.4;",
		"DEVELOPMENT_TEAM = ABCDE12345;",
		"ASSETCATALOG_COMPILER_APPICON_NAME = AppIcon;",
		"AVFoundation,",
		`"$(DERIVED_FILE_DIR)/libapp.a",`,
		"buildmode=c-archive",
		"PrivacyInfo.xcprivacy in Resources */,",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("pbxproj lacks %q", want)
		}
	}
	lintPlist(t, "project.pbxproj", []byte(got))
	if teamID("Apple Development: Jane") != "" {
		t.Error("teamID without a team")
	}
}

func TestPermissionsTable(t *testing.T) {
	for _, p := range permissions {
		if len(p.iosKeys) > 0 && !strings.Contains(p.iosText, "%s") {
			t.Errorf("%s: default iOS text does not name the app", p.name)
		}
		if len(p.android) == 0 && !p.component {
			t.Errorf("%s: no Android permission", p.name)
		}
	}
	perms, _ := androidPermissions(&Config{Permissions: []string{"location", "biometric"}})
	var names []string
	for _, p := range perms {
		names = append(names, p.name)
	}
	if names[0] != "android.permission.INTERNET" || !slices.Contains(names, "android.permission.USE_BIOMETRIC") {
		t.Errorf("perms %v", names)
	}
}

// syncAndroidProject copies the template but none of one machine's files:
// IDE metadata, build outputs, local.properties.
func TestSyncAndroidProjectSkipsLocalFiles(t *testing.T) {
	src, proj := t.TempDir(), t.TempDir()
	write := func(root, rel string) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	keep := []string{
		"build.gradle", "settings.gradle", "gradle.properties", "gradlew",
		"gradle/wrapper/gradle-wrapper.properties",
		"app/build.gradle", "app/src/main/AndroidManifest.xml",
		"app/src/main/java/org/tango/host/Tango.java",
		"app/src/main/res/values/build.xml", // a file named like a skipped dir
	}
	skip := []string{
		".project", ".classpath", ".settings/org.eclipse.buildship.core.prefs",
		"app/.project", "app/.classpath", "app/.settings/org.eclipse.jdt.core.prefs",
		".idea/workspace.xml", "app/.idea/misc.xml", "android.iml", "app/app.iml",
		"local.properties", "app/local.properties",
		"build/tmp/x", "app/build/outputs/apk/debug/app-debug.apk",
		".gradle/8.0/x", ".kotlin/errors/x", "app/.cxx/x",
		"app/src/main/jniLibs/arm64-v8a/libtango.so", "build.sh",
	}
	for _, rel := range append(append([]string(nil), keep...), skip...) {
		write(src, rel)
	}
	// A stale source in the project is replaced, not kept.
	write(proj, "app/src/main/java/Old.java")
	if err := syncAndroidProject(src, proj); err != nil {
		t.Fatal(err)
	}
	for _, rel := range keep {
		if _, err := os.Stat(filepath.Join(proj, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s not copied: %v", rel, err)
		}
	}
	for _, rel := range append(skip, "app/src/main/java/Old.java") {
		if _, err := os.Stat(filepath.Join(proj, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s copied into the project", rel)
		}
	}
	for _, dir := range []string{".settings", "app/.settings", ".idea", "build", "app/build", ".gradle"} {
		if _, err := os.Stat(filepath.Join(proj, filepath.FromSlash(dir))); err == nil {
			t.Errorf("directory %s created in the project", dir)
		}
	}
}

// The host template in this checkout carries no IDE or machine files.
func TestAndroidTemplateHasNoLocalFiles(t *testing.T) {
	root := filepath.Join("..", "..", "mobile", "android")
	if _, err := os.Stat(root); err != nil {
		t.Skip(err)
	}
	out, err := exec.Command("git", "-C", root, "ls-files", ".").Output()
	if err != nil {
		t.Skipf("git ls-files: %v", err)
	}
	for _, rel := range strings.Fields(string(out)) {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			continue // deleted in the work tree, not committed yet
		}
		parts := strings.Split(rel, "/")
		for i, p := range parts {
			if androidLocalFile(p, i < len(parts)-1) {
				t.Errorf("mobile/android/%s is a machine-local file; remove it from the template", rel)
			}
		}
	}
}
