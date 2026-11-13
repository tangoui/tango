package automation

import (
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
)

// Production builds (the default) must not contain the control channel:
// neither the root package nor uihost may depend on it without -tags
// tango_dev, whatever else is set (tango_release is a no-op alias).
func TestReleaseBuildHasNoChannel(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool")
	}
	list := func(tags ...string) []string {
		args := append([]string{"list", "-deps"}, tags...)
		args = append(args, "github.com/tangoui/tango", "github.com/tangoui/tango/uihost")
		out, err := exec.Command(goTool, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("go list: %v\n%s", err, out)
		}
		return strings.Fields(string(out))
	}
	for _, pkg := range []string{"github.com/tangoui/tango/internal/automation", "github.com/tangoui/tango/internal/devhost"} {
		if !slices.Contains(list("-tags", "tango_dev"), pkg) {
			t.Fatalf("development build lacks %s (test is stale)", pkg)
		}
		if slices.Contains(list(), pkg) {
			t.Fatalf("default (production) build links %s", pkg)
		}
		if slices.Contains(list("-tags", "tango_release"), pkg) {
			t.Fatalf("tango_release build links %s", pkg)
		}
	}
}

func TestWithDevTag(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "-tags tango_dev",
		"-race":                  "-tags tango_dev -race",
		"-tags a,b":              "-tags a,b,tango_dev",
		"-tags=a -mod=mod":       "-tags=a,tango_dev -mod=mod",
		"-tags a -tags=b":        "-tags a -tags=b,tango_dev",
		"--tags=tango_dev":       "--tags=tango_dev",
		"-tags tango_release -v": "-tags tango_release,tango_dev -v",
	} {
		if got := strings.Join(withDevTag(strings.Fields(in)), " "); got != want {
			t.Errorf("withDevTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHasChannel(t *testing.T) {
	dir := t.TempDir()
	yes, no := dir+"/yes", dir+"/no"
	os.WriteFile(yes, []byte("\x7fELF..TANGO_AUTOMATION_TOKEN_FILE.."), 0o755)
	os.WriteFile(no, []byte("\x7fELF.. production .."), 0o755)
	if !hasChannel(yes) || hasChannel(no) {
		t.Fatal("hasChannel wrong")
	}
	if _, err := resolveTarget(no, Options{}, dir); err == nil || !strings.Contains(err.Error(), "-tags tango_dev") {
		t.Fatalf("production executable: err %v", err)
	}
}

func TestLaunchSkipsWithoutE2E(t *testing.T) {
	t.Setenv(EnvE2E, "")
	os.Unsetenv(EnvE2E)
	ran := false
	t.Run("inner", func(t *testing.T) {
		Launch(t, "./does-not-exist")
		ran = true
	})
	if ran {
		t.Fatal("Launch did not skip")
	}
}

func TestStateNames(t *testing.T) {
	got := stateNames(a11y.Checked|a11y.Disabled, "!")
	if strings.Join(got, ",") != "!disabled,!checked" {
		t.Fatalf("%v", got)
	}
}

func TestLocatorRefinementCopies(t *testing.T) {
	w := &Window{info: WindowInfo{ID: 1, Title: "W"}}
	base := w.Find(a11y.RoleButton, "OK")
	a := base.WithState(a11y.Focused).Nth(2)
	b := base.WithoutState(a11y.Disabled).WithValue("v")
	if len(base.q.States) != 0 || base.q.Nth != 0 || base.q.Value != nil {
		t.Fatalf("base changed: %+v", base.q)
	}
	if a.q.States[0] != "focused" || a.q.Nth != 2 || b.q.States[0] != "!disabled" || *b.q.Value != "v" {
		t.Fatalf("%+v %+v", a.q, b.q)
	}
	if w.Find(a11y.RoleNone, "x").q.Role != "" {
		t.Fatal("RoleNone should match any role")
	}
	if s := a.String(); s != `window "W": button "OK" [focused] #2` {
		t.Fatalf("%q", s)
	}
}

func TestTailBuffer(t *testing.T) {
	b := &tailBuffer{max: 8}
	b.Write([]byte("hello "))
	b.Write([]byte("world"))
	if b.String() != "lo world" {
		t.Fatalf("%q", b.String())
	}
}

func TestFilterEnv(t *testing.T) {
	got := filterEnv([]string{"A=1", "TANGO_AUTOMATION=x", "TANGO_AUTOMATION_TOKEN_FILE=y", "TANGO_AUTOMATIONX=z"})
	if strings.Join(got, " ") != "A=1 TANGO_AUTOMATIONX=z" {
		t.Fatalf("%v", got)
	}
}
