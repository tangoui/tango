package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// androidToolchain is where the Android tools are.
type androidToolchain struct {
	sdk string
	ndk string
}

// findAndroidSDK returns $ANDROID_HOME, $ANDROID_SDK_ROOT or the SDK's
// default install location, or "".
func findAndroidSDK() string {
	if sdk := firstNonEmpty(os.Getenv("ANDROID_HOME"), os.Getenv("ANDROID_SDK_ROOT")); sdk != "" {
		return sdk
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, d := range []string{"Library/Android/sdk", "Android/Sdk", "AppData/Local/Android/Sdk"} {
		if st, err := os.Stat(filepath.Join(home, d)); err == nil && st.IsDir() {
			return filepath.Join(home, d)
		}
	}
	return ""
}

// findAndroidNDK returns $ANDROID_NDK_HOME or the newest NDK in the SDK.
func findAndroidNDK(sdk string) string {
	if ndk := os.Getenv("ANDROID_NDK_HOME"); ndk != "" {
		return ndk
	}
	if sdk == "" {
		return ""
	}
	vs, _ := filepath.Glob(filepath.Join(sdk, "ndk", "*"))
	slices.SortFunc(vs, func(a, b string) int { return compareVersions(filepath.Base(a), filepath.Base(b)) })
	if len(vs) == 0 {
		return ""
	}
	return vs[len(vs)-1]
}

// compareVersions compares dotted numeric versions ("28.2.13676358").
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		var x, y int
		if i < len(as) {
			x, _ = strconv.Atoi(as[i])
		}
		if i < len(bs) {
			y, _ = strconv.Atoi(bs[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

func androidTools() (androidToolchain, error) {
	t := androidToolchain{sdk: findAndroidSDK()}
	if t.sdk == "" {
		return t, errors.New("Android SDK not found: install Android Studio or the command line tools and set ANDROID_HOME")
	}
	t.ndk = findAndroidNDK(t.sdk)
	if t.ndk == "" {
		return t, fmt.Errorf("no NDK in %s/ndk: install one (sdkmanager \"ndk;28.2.13676358\") or set ANDROID_NDK_HOME", t.sdk)
	}
	return t, nil
}

// clang is the NDK compiler for an ABI and API level.
func (t androidToolchain) clang(goarch string, api int) (string, error) {
	host := map[string]string{"darwin": "darwin-x86_64", "linux": "linux-x86_64", "windows": "windows-x86_64"}[runtime.GOOS]
	if host == "" {
		return "", fmt.Errorf("the NDK does not run on %s", runtime.GOOS)
	}
	var prefix string
	for _, a := range androidABIs {
		if a.goarch == goarch {
			prefix = a.clang
		}
	}
	name := prefix + strconv.Itoa(api) + "-clang"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	cc := filepath.Join(t.ndk, "toolchains", "llvm", "prebuilt", host, "bin", name)
	if _, err := os.Stat(cc); err != nil {
		return "", fmt.Errorf("NDK compiler %s not found (minSdk %d too new or too old for NDK %s?)", cc, api, filepath.Base(t.ndk))
	}
	return cc, nil
}

func (t androidToolchain) adb() string {
	return filepath.Join(t.sdk, "platform-tools", exeName("adb"))
}

func (t androidToolchain) emulator() string {
	return filepath.Join(t.sdk, "emulator", exeName("emulator"))
}

func exeName(s string) string {
	if runtime.GOOS == "windows" {
		return s + ".exe"
	}
	return s
}

// androidRelease is the release signing setup.
type androidRelease struct {
	keystore, alias string
}

func androidReleaseConfig(cfg *Config) (androidRelease, error) {
	a := cfg.Android
	if a.Keystore == "" {
		return androidRelease{}, fmt.Errorf(`a release build is signed with your upload key; tan.json needs "android.keystore" and "android.keyAlias" (create a key with: keytool -genkeypair -v -keystore release.jks -alias upload -keyalg RSA -keysize 2048 -validity 10000), and $%s must hold its password. For a debug APK pass -debug`, envAndroidStorePass)
	}
	r := androidRelease{keystore: cfg.Path(a.Keystore), alias: a.KeyAlias}
	if !keyAliasPattern.MatchString(r.alias) { // also checked by validate
		return r, fmt.Errorf(`"android.keyAlias" %q must be letters, digits, '.', '_' or '-'`, r.alias)
	}
	if _, err := os.Stat(r.keystore); err != nil {
		return r, fmt.Errorf(`"android.keystore": %w`, err)
	}
	if os.Getenv(envAndroidStorePass) == "" {
		return r, fmt.Errorf("set $%s to the password of %s (and $%s if the key's password differs)", envAndroidStorePass, a.Keystore, envAndroidKeyPass)
	}
	return r, nil
}

// checkAndroidBuild fails early when tools or signing are missing.
func checkAndroidBuild(cfg *Config, debug bool) error {
	t, err := androidTools()
	if err != nil {
		return err
	}
	if _, err := t.clang("arm64", cfg.AndroidMinSdk()); err != nil {
		return err
	}
	if err := checkJava(); err != nil {
		return err
	}
	if !debug {
		_, err := androidReleaseConfig(cfg)
		return err
	}
	return nil
}

// checkJava makes sure Gradle will find a JDK.
func checkJava() error {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		if _, err := os.Stat(filepath.Join(home, "bin", exeName("java"))); err != nil {
			return fmt.Errorf("JAVA_HOME=%s has no bin/java", home)
		}
		return nil
	}
	if _, err := exec.LookPath("java"); err != nil {
		return errors.New("Gradle needs a JDK 17 or newer: install one and set JAVA_HOME")
	}
	return nil
}

// buildAndroid builds libtango.so for each architecture, prepares the
// Gradle project in dir/project from the Tango module's mobile/android
// (manifest, icon, app name from tan.json) and runs Gradle offline. Debug
// builds make a debug APK; release builds a signed APK and AAB.
func buildAndroid(ctx context.Context, cfg *Config, arches []string, dir string, debug, dev bool) (artifact, error) {
	a := artifact{target: target{osAndroid, strings.Join(arches, "+")}, dir: dir}
	tools, err := androidTools()
	if err != nil {
		return a, err
	}
	var rel androidRelease
	if !debug {
		if rel, err = androidReleaseConfig(cfg); err != nil {
			return a, err
		}
	}
	tango, err := tangoModuleDir(ctx, cfg)
	if err != nil {
		return a, err
	}
	proj := filepath.Join(dir, "project")
	if err := syncAndroidProject(filepath.Join(tango, "mobile", "android"), proj); err != nil {
		return a, err
	}
	if err := writeAndroidResources(cfg, tools, proj); err != nil {
		return a, err
	}

	jniLibs := filepath.Join(proj, "app", "src", "main", "jniLibs")
	for _, arch := range arches {
		cc, err := tools.clang(arch, cfg.AndroidMinSdk())
		if err != nil {
			return a, err
		}
		abi := androidABI(arch)
		so := filepath.Join(jniLibs, abi, "libtango.so")
		env := []string{"GOOS=android", "GOARCH=" + arch, "CGO_ENABLED=1", "CC=" + cc}
		if arch == "arm" {
			env = append(env, "GOARM=7")
		}
		logf("  go build -buildmode=c-shared (%s)", abi)
		if err := runIn(ctx, cfg.dir, env, "go", mobileGoBuildArgs("c-shared", so, debug, dev)...); err != nil {
			return a, err
		}
		os.Remove(strings.TrimSuffix(so, ".so") + ".h")
	}

	tasks := []string{"assembleDebug"}
	if !debug {
		tasks = []string{"assembleRelease", "bundleRelease"}
	}
	args := append(androidGradleArgs(), tasks...)
	logf("  gradle %s", strings.Join(tasks, " "))
	err = runGradle(ctx, proj, args, gradleEnv(cfg, rel))
	if _, set := os.LookupEnv("TANGO_GRADLE_ARGS"); err != nil && !set && ctx.Err() == nil {
		// The cache may not have the host's dependencies yet (a first
		// build, or a new androidx.webkit): try once online.
		logf("  gradle --offline failed; retrying online in case a dependency is not in the Gradle cache yet")
		err = runGradle(ctx, proj, tasks, gradleEnv(cfg, rel))
	}
	if err != nil {
		return a, err
	}

	outputs := filepath.Join(proj, "app", "build", "outputs")
	base := fmt.Sprintf("%s-%s", cfg.Exe(), cfg.Version)
	if debug {
		a.path = filepath.Join(dir, base+"-debug.apk")
		return a, copyFile(filepath.Join(outputs, "apk", "debug", "app-debug.apk"), a.path, 0o644)
	}
	a.path = filepath.Join(dir, base+".apk")
	if err := copyFile(filepath.Join(outputs, "apk", "release", "app-release.apk"), a.path, 0o644); err != nil {
		return a, err
	}
	aab := filepath.Join(dir, base+".aab")
	if err := copyFile(filepath.Join(outputs, "bundle", "release", "app-release.aab"), aab, 0o644); err != nil {
		return a, err
	}
	a.extra = append(a.extra, aab)
	return a, nil
}

// androidGradleArgs are the Gradle flags before the properties: --offline
// unless $TANGO_GRADLE_ARGS says otherwise (the host needs the Android
// Gradle plugin and androidx.webkit, which are usually cached; when the
// offline build fails, buildAndroid retries once online).
func androidGradleArgs() []string {
	if v, ok := os.LookupEnv("TANGO_GRADLE_ARGS"); ok {
		return strings.Fields(v)
	}
	return []string{"--offline"}
}

// gradleProjectEnv is the prefix of environment variables Gradle turns
// into project properties (ORG_GRADLE_PROJECT_tangoAppId=… is the same as
// -PtangoAppId=…; see Gradle's "Project properties" documentation).
const gradleProjectEnv = "ORG_GRADLE_PROJECT_"

// gradleEnv passes the app's identity and the release key to
// app/build.gradle as project properties in the environment, not as -P
// flags: on Windows gradlew.bat runs under cmd.exe, which would interpret
// characters like & in a key alias or the keystore's path on its command
// line. Passwords also stay in the environment (TANGO_ANDROID_*), which
// build.gradle reads directly.
func gradleEnv(cfg *Config, rel androidRelease) []string {
	props := [][2]string{
		{"tangoAppId", cfg.ID},
		{"tangoVersionName", cfg.Version},
		{"tangoVersionCode", strconv.Itoa(cfg.AndroidVersionCode())},
		{"tangoMinSdk", strconv.Itoa(cfg.AndroidMinSdk())},
		{"tangoTargetSdk", strconv.Itoa(cfg.AndroidTargetSdk())},
		{"tangoCompileSdk", strconv.Itoa(max(cfg.AndroidTargetSdk(), defaultAndroidSdk))},
	}
	if rel.keystore != "" {
		props = append(props, [2]string{"tangoKeystore", rel.keystore}, [2]string{"tangoKeyAlias", rel.alias})
	}
	env := make([]string, len(props))
	for i, p := range props {
		env[i] = gradleProjectEnv + p[0] + "=" + p[1]
	}
	return env
}

// cmdSpecial are the characters cmd.exe gives a meaning on a batch file's
// command line or in an unquoted variable expansion (gradlew.bat expands
// its own directory unquoted: set DIRNAME=%~dp0).
const cmdSpecial = "&|<>^%!\"()"

// gradleCommand is the Gradle wrapper command for goos, run in proj.
//
// On Windows gradlew.bat is run directly, not through "cmd /c". Go's
// os/exec quotes arguments for CommandLineToArgvW, not for cmd.exe, which
// CreateProcess starts for a batch file (os/exec documentation of
// Command); so nothing tan.json controls goes on the command line (see
// gradleEnv), the remaining arguments ($TANGO_GRADLE_ARGS and the task
// names) must be free of cmd.exe's special characters, and the project
// directory, which gradlew.bat expands unquoted, must not contain & | < >
// or ^. Path is absolute and Args[0] the bare name, so the project's path
// is not part of the command line either.
func gradleCommand(ctx context.Context, goos, proj string, args []string) (*exec.Cmd, error) {
	if goos != "windows" {
		// sh, because module-cache copies of gradlew may have lost +x.
		cmd := exec.CommandContext(ctx, "sh", append([]string{"gradlew"}, args...)...)
		cmd.Dir = proj
		return cmd, nil
	}
	if strings.ContainsAny(proj, "&|<>^") {
		return nil, fmt.Errorf("the build directory %s contains one of & | < > ^, which gradlew.bat cannot run from safely: move the project to a path without them", proj)
	}
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, cmdSpecial+" \t\r\n") {
			return nil, fmt.Errorf("Gradle argument %q: on Windows the arguments ($TANGO_GRADLE_ARGS) must not contain spaces or any of %s", a, cmdSpecial)
		}
	}
	cmd := exec.CommandContext(ctx, filepath.Join(proj, "gradlew.bat"), args...)
	cmd.Args[0] = "gradlew.bat"
	cmd.Dir = proj
	return cmd, nil
}

// execGradle runs the Gradle command with env added. Tests replace it.
var execGradle = func(cmd *exec.Cmd, env []string) error { return runCmd(cmd, cmd.Dir, env) }

func runGradle(ctx context.Context, proj string, args, env []string) error {
	cmd, err := gradleCommand(ctx, runtime.GOOS, proj, args)
	if err != nil {
		return err
	}
	return execGradle(cmd, env)
}

// syncAndroidProject copies the host project into proj, replacing sources
// from an earlier build but keeping Gradle's caches (.gradle, app/build).
func syncAndroidProject(src, proj string) error {
	for _, d := range []string{"app/src", "gradle"} {
		if err := os.RemoveAll(filepath.Join(proj, d)); err != nil {
			return err
		}
	}
	return copyTree(src, proj, func(rel string, d fs.DirEntry) bool {
		switch rel {
		case ".gradle", "build", "app/build", "app/src/main/jniLibs", "local.properties", ".idea", ".kotlin", "build.sh":
			return true
		}
		return false
	})
}

// writeAndroidResources writes local.properties, the app name, the
// launcher icon and the manifest into the copied project.
func writeAndroidResources(cfg *Config, tools androidToolchain, proj string) error {
	// Gradle reads the SDK location from here when ANDROID_HOME is unset.
	sdkDir := strings.NewReplacer(`\`, `\\`, ":", `\:`).Replace(tools.sdk)
	if err := os.WriteFile(filepath.Join(proj, "local.properties"), []byte("sdk.dir="+sdkDir+"\n"), 0o644); err != nil {
		return err
	}
	res := filepath.Join(proj, "app", "src", "main", "res")
	name := fmt.Sprintf("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<!-- Generated by tan build from tan.json. -->\n<resources>\n    <string name=\"app_name\">%s</string>\n</resources>\n", androidString(cfg.Name))
	if err := os.MkdirAll(filepath.Join(res, "values"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(res, "values", "tango_app.xml"), []byte(name), 0o644); err != nil {
		return err
	}
	img, err := loadIcon(cfg)
	if err != nil {
		return err
	}
	// Launcher (legacy + adaptive) and notification icons: icon_mobile.go.
	if err := writeAndroidIcons(cfg, img, res); err != nil {
		return err
	}
	manifestPath := filepath.Join(proj, "app", "src", "main", "AndroidManifest.xml")
	tmpl, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	manifest, err := androidManifest(string(tmpl), cfg, img != nil)
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, []byte(manifest), 0o644)
}

// androidString escapes s for an Android string resource: XML escaping,
// plus backslash escapes for the characters aapt treats specially.
func androidString(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `'`, `\'`, `"`, `\"`, "\n", `\n`).Replace(s)
	if strings.HasPrefix(s, "@") || strings.HasPrefix(s, "?") {
		s = `\` + s
	}
	return html.EscapeString(s)
}

// hostActivity is the Activity tan declares the deep links on.
const hostActivity = "org.tango.host.TangoActivity"

// androidManifest adds tan.json's settings to the host's manifest
// template: permissions and optional hardware features before
// <application>, the deep-link intent filter in the host Activity, and the
// launcher icon. Everything else in the template is kept, so the host can
// declare its own components.
func androidManifest(tmpl string, cfg *Config, icon bool) (string, error) {
	appAt := strings.Index(tmpl, "<application")
	if appAt < 0 {
		return "", errors.New("AndroidManifest.xml template has no <application>")
	}
	perms, features := androidPermissions(cfg)
	var head strings.Builder
	for _, p := range perms {
		if strings.Contains(tmpl, `"`+p.name+`"`) {
			continue
		}
		if p.maxSdk > 0 {
			fmt.Fprintf(&head, "    <uses-permission android:name=\"%s\" android:maxSdkVersion=\"%d\" />\n", p.name, p.maxSdk)
		} else {
			fmt.Fprintf(&head, "    <uses-permission android:name=\"%s\" />\n", p.name)
		}
	}
	for _, f := range features {
		if !strings.Contains(tmpl, `"`+f+`"`) {
			fmt.Fprintf(&head, "    <uses-feature android:name=\"%s\" android:required=\"false\" />\n", f)
		}
	}
	if head.Len() > 0 {
		tmpl = tmpl[:appAt] + "<!-- From tan.json \"permissions\". -->\n" + head.String() + "\n    " + tmpl[appAt:]
	}

	if icon || cfg.Icons.Notification != "" {
		appAt = strings.Index(tmpl, "<application")
		end := appAt + strings.Index(tmpl[appAt:], ">")
		app := tmpl[appAt:end]
		if icon {
			app = setXMLAttr(app, "android:icon", "@mipmap/ic_launcher")
			app = setXMLAttr(app, "android:roundIcon", "@mipmap/ic_launcher_round")
		}
		// FCM shows notifications it receives in the background with this
		// icon; without it, a silhouette of the launcher icon.
		var meta string
		if cfg.Icons.Notification != "" && !strings.Contains(tmpl, `"`+androidFCMIconMeta+`"`) {
			meta = fmt.Sprintf("\n        <!-- tan.json \"icons.notification\". -->\n        <meta-data\n            android:name=\"%s\"\n            android:resource=\"@drawable/%s\" />", androidFCMIconMeta, androidNotifyName)
		}
		tmpl = tmpl[:appAt] + app + ">" + meta + tmpl[end+1:]
	}

	if len(cfg.URLSchemes) > 0 {
		act := strings.Index(tmpl, `"`+hostActivity+`"`)
		if act < 0 {
			return "", fmt.Errorf("AndroidManifest.xml template does not declare %s", hostActivity)
		}
		end := strings.Index(tmpl[act:], "</activity>")
		if end < 0 {
			return "", fmt.Errorf("AndroidManifest.xml: %s has no </activity>", hostActivity)
		}
		end += act
		var f strings.Builder
		f.WriteString("    <!-- Deep links: tan.json \"urlSchemes\". -->\n")
		f.WriteString("            <intent-filter>\n")
		f.WriteString("                <action android:name=\"android.intent.action.VIEW\" />\n")
		f.WriteString("                <category android:name=\"android.intent.category.DEFAULT\" />\n")
		f.WriteString("                <category android:name=\"android.intent.category.BROWSABLE\" />\n")
		for _, sc := range cfg.URLSchemes {
			fmt.Fprintf(&f, "                <data android:scheme=\"%s\" />\n", sc)
		}
		f.WriteString("            </intent-filter>\n        ")
		tmpl = tmpl[:end] + f.String() + tmpl[end:]
	}

	// App Links: an autoVerify filter per "appLinks" domain (applinks.go).
	tmpl, err := androidAppLinkFilters(tmpl, cfg)
	if err != nil {
		return "", err
	}

	if hasPermission(cfg, "widget") && !strings.Contains(tmpl, `"`+hostWidget+`"`) {
		end := strings.LastIndex(tmpl, "</application>")
		if end < 0 {
			return "", errors.New("AndroidManifest.xml template has no </application>")
		}
		tmpl = tmpl[:end] + androidWidgetReceiver + tmpl[end:]
	}
	return tmpl, nil
}

// hostWidget is the host's AppWidgetProvider (sys/widget), declared when
// tan.json "permissions" lists "widget".
const hostWidget = "org.tango.host.TangoWidget"

const androidWidgetReceiver = `    <!-- Home-screen widget: tan.json "permissions" lists "widget". -->
        <receiver
            android:name="` + hostWidget + `"
            android:label="@string/app_name"
            android:exported="false">
            <intent-filter>
                <action android:name="android.appwidget.action.APPWIDGET_UPDATE" />
            </intent-filter>
            <meta-data
                android:name="android.appwidget.provider"
                android:resource="@xml/tango_widget_info" />
        </receiver>
    `
