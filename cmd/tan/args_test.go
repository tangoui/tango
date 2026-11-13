package main

import (
	"strings"
	"testing"
)

// Commands that take only flags must not ignore a positional argument:
// "tan build ios" used to build the desktop app.
func TestUnexpectedPositionalArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func([]string) error
		args []string
		want string
	}{
		{"build", runBuild, []string{"ios"}, `unexpected argument "ios": did you mean -platform ios?`},
		{"build", runBuild, []string{"-debug", "android/arm64"}, "did you mean -platform android/arm64?"},
		{"build", runBuild, []string{"-C", "x", "foo"}, `unexpected argument "foo"`},
		{"run", runRun, []string{"android"}, "did you mean -platform android?"},
		{"run", runRun, []string{"ios-simulator"}, "did you mean -platform ios-simulator?"},
		{"package", runPackage, []string{"linux"}, "did you mean -platform linux?"},
		{"dev", runDev, []string{"android"}, "did you mean -platform android?"},
		{"dev", runDev, []string{"-hot=false", "x"}, `unexpected argument "x"`},
		{"doctor", runDoctor, []string{"x"}, `unexpected argument "x"`},
		{"applinks", runAppLinks, []string{"ios"}, "did you mean -platform ios?"},
		{"keygen", runKeygen, []string{"x"}, `unexpected argument "x"`},
	} {
		err := tc.run(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("tan %s %q: err = %v, want %q", tc.name, tc.args, err, tc.want)
		}
		if err != nil && !strings.Contains(tc.want, "did you mean") && strings.Contains(err.Error(), "did you mean") {
			t.Errorf("tan %s %q: unexpected hint in %v", tc.name, tc.args, err)
		}
	}
}

// tan dev passes arguments after "--" on to the app.
func TestDevArgsAfterDashDash(t *testing.T) {
	for _, tc := range []struct {
		args []string
		ok   bool
	}{
		{nil, true},
		{[]string{"-hot=false"}, true},
		{[]string{"--", "a", "b"}, true},
		{[]string{"-C", "x", "--", "ios"}, true},
		{[]string{"--", "--"}, true},
		{[]string{"a"}, false},
		{[]string{"-C", "x", "a", "--", "b"}, false},
	} {
		fs := newFlags("dev", "")
		fs.SetOutput(new(strings.Builder))
		projectFlag(fs)
		fs.Bool("hot", true, "")
		if err := fs.Parse(tc.args); err != nil {
			t.Fatal(err)
		}
		if err := argsAfterDashDash(fs, tc.args); (err == nil) != tc.ok {
			t.Errorf("%q: err = %v, want ok=%v", tc.args, err, tc.ok)
		}
	}
}
