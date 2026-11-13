package env

import "testing"

func TestOSReleaseName(t *testing.T) {
	for in, want := range map[string]string{
		"NAME=\"Ubuntu\"\nVERSION=\"24.04 LTS\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n": "Ubuntu 24.04.1 LTS",
		"# c\nNAME=Fedora\nVERSION='41 (Workstation)'\n":                               "Fedora 41 (Workstation)",
		"": "",
	} {
		if got := osReleaseName(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
