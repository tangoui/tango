package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// iosSDK describes the SDK a build compiles against.
type iosSDK struct {
	device   bool
	name     string // iphoneos | iphonesimulator
	platform string // CFBundleSupportedPlatforms: iPhoneOS | iPhoneSimulator
	path     string // sysroot
	version  string // e.g. 26.1
	build    string // SDK build number
	clang    string
}

func (s iosSDK) target(minOS string) string {
	t := "arm64-apple-ios" + minOS
	if !s.device {
		t += "-simulator"
	}
	return t
}

func lookupIOSSDK(ctx context.Context, device bool) (iosSDK, error) {
	s := iosSDK{device: device, name: "iphonesimulator", platform: "iPhoneSimulator"}
	if device {
		s.name, s.platform = "iphoneos", "iPhoneOS"
	}
	q := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "xcrun", append([]string{"--sdk", s.name}, args...)...).Output()
		return strings.TrimSpace(string(out)), err
	}
	var err error
	if s.path, err = q("--show-sdk-path"); err != nil {
		return s, fmt.Errorf("no %s SDK: install Xcode and run xcode-select -s /Applications/Xcode.app", s.name)
	}
	if s.clang, err = q("--find", "clang"); err != nil {
		return s, fmt.Errorf("no clang for %s: %v", s.name, err)
	}
	s.version, _ = q("--show-sdk-version")
	s.build, _ = q("--show-sdk-build-version")
	return s, nil
}

// iosSigning is what a device build is signed with.
type iosSigning struct {
	identity     string
	profile      string // .mobileprovision
	entitlements string // plist, or "" to take them from the profile
}

func iosSigningConfig(cfg *Config) (iosSigning, error) {
	s := iosSigning{identity: firstNonEmpty(os.Getenv(envIOSSignIdentity), cfg.IOS.SignIdentity)}
	var missing []string
	if s.identity == "" {
		missing = append(missing, `"ios.signIdentity" (or $`+envIOSSignIdentity+`), e.g. "Apple Development: Jane Doe (TEAMID)"; list yours with: security find-identity -v -p codesigning`)
	}
	if cfg.IOS.ProvisioningProfile == "" {
		missing = append(missing, `"ios.provisioningProfile", the .mobileprovision for `+cfg.ID+` from developer.apple.com (or Xcode's ~/Library/Developer/Xcode/UserData/Provisioning Profiles)`)
	} else {
		s.profile = cfg.Path(cfg.IOS.ProvisioningProfile)
		if _, err := os.Stat(s.profile); err != nil {
			return s, fmt.Errorf(`"ios.provisioningProfile": %w`, err)
		}
	}
	if len(missing) > 0 {
		return s, fmt.Errorf("a device build must be signed; tan.json needs:\n  - %s\n(or build for the Simulator with -platform ios-simulator)", strings.Join(missing, "\n  - "))
	}
	if cfg.IOS.Entitlements != "" {
		s.entitlements = cfg.Path(cfg.IOS.Entitlements)
		if _, err := os.Stat(s.entitlements); err != nil {
			return s, fmt.Errorf(`"ios.entitlements": %w`, err)
		}
	}
	return s, nil
}

// checkIOSBuild fails early when the host cannot build t.
func checkIOSBuild(cfg *Config, t target, debug bool) error {
	if runtime.GOOS != "darwin" {
		return errors.New("iOS apps can only be built on macOS with Xcode")
	}
	if _, err := exec.LookPath("xcrun"); err != nil {
		return errors.New("xcrun not found: install Xcode")
	}
	if t.OS == osIOS {
		_, err := iosSigningConfig(cfg)
		return err
	}
	return nil
}

// buildIOS builds the .app (and, for devices, the .ipa) into dir:
//
//  1. go build -buildmode=c-archive with the SDK's clang;
//  2. link the native host (mobile/ios/main.m) with the archive;
//  3. Info.plist from tan.json, the app icon (actool), code signature.
func buildIOS(ctx context.Context, cfg *Config, t target, dir string, debug, dev bool) (artifact, error) {
	a := artifact{target: t, dir: dir}
	device := t.OS == osIOS
	sdk, err := lookupIOSSDK(ctx, device)
	if err != nil {
		return a, err
	}
	var sign iosSigning
	if device {
		if sign, err = iosSigningConfig(cfg); err != nil {
			return a, err
		}
	}
	tango, err := tangoModuleDir(ctx, cfg)
	if err != nil {
		return a, err
	}
	if err := os.RemoveAll(dir); err != nil {
		return a, err
	}
	work := filepath.Join(dir, "obj")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return a, err
	}
	defer os.RemoveAll(work)

	minOS := cfg.IOSMinVersion()
	triple := sdk.target(minOS)
	env := []string{
		"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1", "CC=" + sdk.clang,
		"CGO_CFLAGS=-isysroot " + sdk.path + " -target " + triple + " -O2",
		"CGO_LDFLAGS=-isysroot " + sdk.path + " -target " + triple,
	}
	lib := filepath.Join(work, "lib"+cfg.Exe()+".a")
	logf("  go build -buildmode=c-archive (%s)", triple)
	if err := runIn(ctx, cfg.dir, env, "go", mobileGoBuildArgs("c-archive", lib, debug, dev)...); err != nil {
		return a, err
	}
	extraFlags, err := cgoLinkFlags(ctx, cfg, env)
	if err != nil {
		return a, err
	}

	a.path = filepath.Join(dir, cfg.Name+".app")
	a.exe = filepath.Join(a.path, cfg.Exe())
	if err := os.MkdirAll(a.path, 0o755); err != nil {
		return a, err
	}
	logf("  link %s", cfg.Exe())
	link := iosLinkArgs(sdk, triple, filepath.Join(tango, "mobile", "ios", "main.m"), lib, a.exe, extraFlags, debug)
	if !device {
		// Simulator apps carry their entitlements in a section of the
		// executable, as Xcode does; without them the Keychain fails with
		// errSecMissingEntitlement.
		ent := filepath.Join(work, "simulator.entitlements")
		if err := os.WriteFile(ent, encodePlist(simulatorEntitlements(cfg)), 0o644); err != nil {
			return a, err
		}
		link = append(link, "-Wl,-sectcreate,__TEXT,__entitlements,"+ent)
	}
	if err := runIn(ctx, "", nil, sdk.clang, link...); err != nil {
		return a, err
	}

	icons, err := iosAppIcon(ctx, cfg, sdk, minOS, a.path, work)
	if err != nil {
		warnf("App icon skipped: %v", err)
	}
	info := iosInfoPlist(cfg, sdk, icons)
	if device {
		info = append(info, iosBuildInfo(ctx, sdk)...)
	}
	if err := os.WriteFile(filepath.Join(a.path, "Info.plist"), encodePlist(info), 0o644); err != nil {
		return a, err
	}
	if err := os.WriteFile(filepath.Join(a.path, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return a, err
	}
	// Tango's privacy manifest (required-reason APIs the runtime uses).
	if privacy := filepath.Join(tango, "mobile", "ios", "PrivacyInfo.xcprivacy"); fileExists(privacy) {
		if err := copyFile(privacy, filepath.Join(a.path, "PrivacyInfo.xcprivacy"), 0o644); err != nil {
			return a, err
		}
	}

	if !device {
		// The Simulator runs ad-hoc signed apps.
		return a, runIn(ctx, "", nil, "codesign", "--force", "--sign", "-", "--timestamp=none", a.path)
	}
	if err := copyFile(sign.profile, filepath.Join(a.path, "embedded.mobileprovision"), 0o644); err != nil {
		return a, err
	}
	ent := sign.entitlements
	if ent == "" {
		ent = filepath.Join(work, "entitlements.plist")
		if err := profileEntitlements(ctx, sign.profile, cfg.ID, ent); err != nil {
			return a, err
		}
	}
	if len(cfg.AppLinks) > 0 {
		// Universal Links: the profile's (or ios.entitlements') entitlements
		// plus applinks:<domain> for each "appLinks" domain.
		merged := filepath.Join(work, "entitlements.applinks.plist")
		if err := writeAppLinkEntitlements(ctx, cfg, ent, merged); err != nil {
			return a, err
		}
		warnProfileAppLinks(ctx, cfg, sign.profile)
		ent = merged
	}
	logf("  codesign (%s)", sign.identity)
	if err := runIn(ctx, "", nil, "codesign", "--force", "--sign", sign.identity, "--entitlements", ent, "--timestamp=none", a.path); err != nil {
		return a, err
	}
	ipa := filepath.Join(dir, fmt.Sprintf("%s-%s.ipa", cfg.Exe(), cfg.Version))
	if err := writeIPA(ipa, a.path); err != nil {
		return a, err
	}
	a.extra = append(a.extra, ipa)
	return a, nil
}

// mobileGoBuildArgs builds the main package as a C library. Like desktop
// builds, every tan build (debug included) is a production build: the
// user's own tags without tango_dev (build_tags.go). Only tan dev (dev set)
// adds tango_dev, which links the development features. Debug builds keep
// symbols and paths.
func mobileGoBuildArgs(mode, out string, debug, dev bool) []string {
	args := []string{"build", "-buildmode=" + mode, "-tags", buildTags(dev)}
	if !debug {
		args = append(args, "-trimpath", "-ldflags", "-s -w")
	}
	return append(args, "-o", out, ".")
}

// simulatorEntitlements are linked into Simulator builds: the app
// identifier and a Keychain access group named after it, prefixed with the
// team when ios.signIdentity names one; and the "appLinks" associated
// domains.
func simulatorEntitlements(cfg *Config) plist {
	appID := cfg.ID
	if team := teamID(firstNonEmpty(os.Getenv(envIOSSignIdentity), cfg.IOS.SignIdentity)); team != "" {
		appID = team + "." + cfg.ID
	}
	ent := plist{
		{"application-identifier", appID},
		{"keychain-access-groups", []string{appID}},
	}
	if len(cfg.AppLinks) > 0 {
		ent = withAssociatedDomains(ent, cfg.associatedDomains())
	}
	return ent
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// iosLinkArgs links the host's main.m with the Go archive. -ObjC keeps
// Objective-C classes only named at run time (TangoSceneDelegate, from
// Info.plist); Go's net package needs libresolv. extra are the #cgo
// LDFLAGS of the Go packages, which the archive does not record.
func iosLinkArgs(sdk iosSDK, triple, mainM, lib, out string, extra []string, debug bool) []string {
	args := []string{"-isysroot", sdk.path, "-target", triple, "-fobjc-arc", "-O2"}
	if debug {
		args = append(args, "-g")
	}
	args = append(args, "-o", out, mainM, lib, "-Wl,-ObjC", "-lresolv")
	base := []string{"-framework", "Foundation", "-framework", "UIKit", "-framework", "WebKit",
		"-framework", "UniformTypeIdentifiers", "-framework", "CoreFoundation", "-framework", "Security"}
	return append(args, dedupeLinkFlags(append(base, extra...))...)
}

// iosInfoPlist renders the app's Info.plist. icons holds the keys actool
// produced for the app icon (CFBundleIcons...), if any.
func iosInfoPlist(cfg *Config, sdk iosSDK, icons map[string]any) plist {
	p := plist{
		{"CFBundleDevelopmentRegion", "en"},
		// UIKit localizes system-provided UI (edit menu, alerts' system
		// buttons, ...) only into declared languages.
		{"CFBundleLocalizations", cfg.AppleLocalizations()},
		{"CFBundleDisplayName", cfg.Name},
		{"CFBundleExecutable", cfg.Exe()},
		{"CFBundleIdentifier", cfg.ID},
		{"CFBundleInfoDictionaryVersion", "6.0"},
		{"CFBundleName", cfg.Name},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleShortVersionString", cfg.IOSShortVersion()},
		{"CFBundleVersion", cfg.IOSBuild()},
		{"CFBundleSupportedPlatforms", []string{sdk.platform}},
		{"MinimumOSVersion", cfg.IOSMinVersion()},
		{"LSRequiresIPhoneOS", true},
		{"UIDeviceFamily", []any{1, 2}},
		{"UIRequiredDeviceCapabilities", []string{"arm64"}},
		// Without a launch screen iOS runs the app letterboxed.
		{"UILaunchScreen", plist{}},
		// Lets CADisplayLink run above 60 Hz on ProMotion iPhones (the
		// canvas pages ask for up to 120 Hz); no effect elsewhere.
		{"CADisableMinimumFrameDurationOnPhone", true},
		// TangoSceneDelegate (in the Go archive) creates the window.
		{"UIApplicationSceneManifest", plist{
			{"UIApplicationSupportsMultipleScenes", false},
			{"UISceneConfigurations", plist{
				{"UIWindowSceneSessionRoleApplication", []any{plist{
					{"UISceneConfigurationName", "Default"},
					{"UISceneDelegateClassName", "TangoSceneDelegate"},
				}}},
			}},
		}},
		{"UISupportedInterfaceOrientations", []string{
			"UIInterfaceOrientationPortrait", "UIInterfaceOrientationLandscapeLeft", "UIInterfaceOrientationLandscapeRight"}},
		{"UISupportedInterfaceOrientations~ipad", []string{
			"UIInterfaceOrientationPortrait", "UIInterfaceOrientationPortraitUpsideDown",
			"UIInterfaceOrientationLandscapeLeft", "UIInterfaceOrientationLandscapeRight"}},
	}
	if cfg.Copyright != "" {
		p.set("NSHumanReadableCopyright", cfg.Copyright)
	}
	if len(cfg.URLSchemes) > 0 {
		p.set("CFBundleURLTypes", []any{plist{
			{"CFBundleURLName", cfg.ID},
			{"CFBundleTypeRole", "Editor"},
			{"CFBundleURLSchemes", cfg.URLSchemes},
		}})
	}
	usage := iosUsageDescriptions(cfg)
	for _, k := range sortedKeys(usage) {
		p.set(k, usage[k])
	}
	if hasPermission(cfg, "background") {
		// sys/background: the iOS backend registers these two BGTask
		// identifiers at launch (internal/platform/ios/mobile2.m).
		p.set("UIBackgroundModes", []string{"fetch", "processing"})
		p.set("BGTaskSchedulerPermittedIdentifiers", []string{cfg.ID + ".tango.refresh", cfg.ID + ".tango.processing"})
	}
	for _, k := range sortedKeys(icons) {
		p.set(k, icons[k])
	}
	return p
}

// iosBuildInfo adds the DT* keys Xcode writes; App Store Connect checks
// them when an .ipa is uploaded.
func iosBuildInfo(ctx context.Context, sdk iosSDK) plist {
	p := plist{
		{"DTPlatformName", sdk.name},
		{"DTPlatformVersion", sdk.version},
		{"DTSDKName", sdk.name + sdk.version},
		{"DTSDKBuild", sdk.build},
		{"DTCompiler", "com.apple.compilers.llvm.clang.1_0"},
	}
	if out, err := exec.CommandContext(ctx, "xcodebuild", "-version").Output(); err == nil {
		// "Xcode 26.1\nBuild version 17B55"
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(lines) == 2 {
			v := strings.TrimPrefix(lines[0], "Xcode ")
			parts := append(strings.Split(v, "."), "0", "0")
			major := parts[0]
			if len(major) == 1 {
				major = "0" + major
			}
			p.set("DTXcode", major+parts[1]+parts[2][:1])
			p.set("DTXcodeBuild", strings.TrimPrefix(lines[1], "Build version "))
			p.set("DTPlatformBuild", sdk.build)
		}
	}
	if out, err := exec.CommandContext(ctx, "sw_vers", "-buildVersion").Output(); err == nil {
		p.set("BuildMachineOSBuild", strings.TrimSpace(string(out)))
	}
	return p
}

// iosAppIcon compiles the project icon into the app's asset catalog
// (Assets.car) with actool and returns the Info.plist keys actool asks
// for. The catalog is written by writeIOSAssetCatalog (icon_mobile.go),
// as for tan build -xcode. When actool rejects the iOS 18 dark/tinted
// variants (an older Xcode), the icon is compiled again without them.
func iosAppIcon(ctx context.Context, cfg *Config, sdk iosSDK, minOS, app, work string) (map[string]any, error) {
	catalog := filepath.Join(work, "Assets.xcassets")
	icon, variants, err := writeIOSAssetCatalog(cfg, catalog, iosIconAppearancesSupported(ctx, cfg))
	if err != nil || !icon {
		return nil, err
	}
	partial := filepath.Join(work, "assets-info.plist")
	actool := func() error {
		out, err := exec.CommandContext(ctx, "xcrun", "actool",
			"--compile", app, "--platform", sdk.name, "--minimum-deployment-target", minOS,
			"--app-icon", "AppIcon", "--output-partial-info-plist", partial,
			"--target-device", "iphone", "--target-device", "ipad",
			"--errors", "--output-format", "human-readable-text",
			catalog).CombinedOutput()
		if err != nil {
			return fmt.Errorf("actool: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	if err := actool(); err != nil {
		if !variants {
			return nil, err
		}
		warnf("The iOS 18 dark/tinted app icons (icons.ios) were left out: %v", err)
		if _, _, err := writeIOSAssetCatalog(cfg, catalog, false); err != nil {
			return nil, err
		}
		if err := actool(); err != nil {
			return nil, err
		}
	}
	js, err := exec.CommandContext(ctx, "plutil", "-convert", "json", "-o", "-", partial).Output()
	if err != nil {
		return nil, fmt.Errorf("plutil: %v", err)
	}
	var keys map[string]any
	if err := json.Unmarshal(js, &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// profileEntitlements writes the entitlements a provisioning profile
// grants (what Xcode signs with) to out, after checking that the profile
// covers bundleID.
func profileEntitlements(ctx context.Context, profile, bundleID, out string) error {
	decoded := out + ".profile.plist"
	defer os.Remove(decoded)
	if b, err := exec.CommandContext(ctx, "security", "cms", "-D", "-i", profile, "-o", decoded).CombinedOutput(); err != nil {
		return fmt.Errorf("reading provisioning profile %s: %v: %s", profile, err, b)
	}
	appID, err := exec.CommandContext(ctx, "plutil", "-extract", "Entitlements.application-identifier", "raw", "-o", "-", decoded).Output()
	if err != nil {
		return fmt.Errorf("provisioning profile %s has no application-identifier", profile)
	}
	if !profileCovers(strings.TrimSpace(string(appID)), bundleID) {
		return fmt.Errorf("provisioning profile %s is for %s, not %s (tan.json \"id\")", profile, strings.TrimSpace(string(appID)), bundleID)
	}
	if b, err := exec.CommandContext(ctx, "plutil", "-extract", "Entitlements", "xml1", "-o", out, decoded).CombinedOutput(); err != nil {
		return fmt.Errorf("extracting entitlements: %v: %s", err, b)
	}
	return nil
}

// profileCovers reports whether a profile's application-identifier
// ("TEAMID.com.example.app" or "TEAMID.*", "TEAMID.com.example.*")
// matches a bundle id.
func profileCovers(appID, bundleID string) bool {
	_, pattern, ok := strings.Cut(appID, ".")
	if !ok {
		return false
	}
	if prefix, wild := strings.CutSuffix(pattern, "*"); wild {
		return strings.HasPrefix(bundleID, prefix)
	}
	return pattern == bundleID
}

// writeIPA zips the app as Payload/<App>.app, keeping file modes. A file
// system without execute bits (Windows) loses them, so the main executable
// and every Mach-O binary are marked executable in the archive.
func writeIPA(ipa, app string) error {
	f, err := os.Create(ipa)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	base := filepath.Dir(app)
	mainExe := filepath.Join(app, strings.TrimSuffix(filepath.Base(app), ".app"))
	err = filepath.Walk(app, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		name := "Payload/" + filepath.ToSlash(rel)
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = name
		if info.IsDir() {
			hdr.Name += "/"
			_, err := zw.CreateHeader(hdr)
			return err
		}
		hdr.Method = zip.Deflate
		if info.Mode().IsRegular() && info.Mode()&0o111 == 0 && (p == mainExe || isMachO(p)) {
			hdr.SetMode(info.Mode() | 0o755)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// isMachO reports whether the file at p starts with a Mach-O or universal
// (fat) binary magic number.
func isMachO(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	var b [4]byte
	if _, err := io.ReadFull(f, b[:]); err != nil {
		return false
	}
	switch string(b[:]) {
	case "\xfe\xed\xfa\xce", "\xfe\xed\xfa\xcf", "\xce\xfa\xed\xfe", "\xcf\xfa\xed\xfe", "\xca\xfe\xba\xbe", "\xbe\xba\xfe\xca":
		return true
	}
	return false
}
