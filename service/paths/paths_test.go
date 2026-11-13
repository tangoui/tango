package paths

import (
	"path/filepath"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDirsFor(t *testing.T) {
	j := filepath.Join
	const id = "com.example.app"
	tests := []struct {
		goos string
		home string
		env  map[string]string
		want Dirs
	}{
		{"darwin", "/Users/u", nil, Dirs{
			Config: j("/Users/u/Library/Application Support", id),
			Data:   j("/Users/u/Library/Application Support", id),
			Cache:  j("/Users/u/Library/Caches", id),
			Logs:   j("/Users/u/Library/Logs", id),
		}},
		{"linux", "/home/u", nil, Dirs{
			Config: j("/home/u/.config", id),
			Data:   j("/home/u/.local/share", id),
			Cache:  j("/home/u/.cache", id),
			Logs:   j("/home/u/.local/state", id, "logs"),
		}},
		{"freebsd", "/home/u", map[string]string{"XDG_CONFIG_HOME": "/x/cfg", "XDG_CACHE_HOME": "relative/ignored"}, Dirs{
			Config: j("/x/cfg", id),
			Data:   j("/home/u/.local/share", id),
			Cache:  j("/home/u/.cache", id),
			Logs:   j("/home/u/.local/state", id, "logs"),
		}},
		{"ios", "/var/mobile/C", nil, Dirs{
			Config: j("/var/mobile/C/Library/Application Support"),
			Data:   j("/var/mobile/C/Library/Application Support/Data"),
			Cache:  j("/var/mobile/C/Library/Caches"),
			Logs:   j("/var/mobile/C/Library/Caches/Logs"),
		}},
		{"android", "/data/user/0/x/files", nil, Dirs{
			Config: j("/data/user/0/x/files/config"),
			Data:   j("/data/user/0/x/files/data"),
			Cache:  j("/data/user/0/x/files/cache"),
			Logs:   j("/data/user/0/x/files/logs"),
		}},
	}
	if filepath.Separator == '\\' {
		tests = append(tests[:0], tests[len(tests):]...) // Unix layouts use host joins; skip on Windows hosts
	}
	tests = append(tests, struct {
		goos string
		home string
		env  map[string]string
		want Dirs
	}{"windows", `C:\Users\u`, map[string]string{"APPDATA": `C:\Users\u\AppData\Roaming`}, Dirs{
		Config: `C:\Users\u\AppData\Roaming\` + id,
		Data:   `C:\Users\u\AppData\Local\` + id,
		Cache:  `C:\Users\u\AppData\Local\` + id + `\Cache`,
		Logs:   `C:\Users\u\AppData\Local\` + id + `\Logs`,
	}})
	for _, tt := range tests {
		got, err := dirsFor(tt.goos, id, tt.home, env(tt.env))
		if err != nil {
			t.Errorf("%s: %v", tt.goos, err)
			continue
		}
		if got != tt.want {
			t.Errorf("%s:\n got %+v\nwant %+v", tt.goos, got, tt.want)
		}
	}
}

func TestErrors(t *testing.T) {
	for _, id := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := dirsFor("linux", id, "/home/u", env(nil)); err == nil {
			t.Errorf("id %q: no error", id)
		}
	}
	for _, goos := range []string{"darwin", "linux", "ios", "android", "windows"} {
		if _, err := dirsFor(goos, "id", "", env(nil)); err != errNoHome {
			t.Errorf("%s without home: %v", goos, err)
		}
	}
	if _, err := dirsFor("linux", "id", "", env(map[string]string{
		"XDG_CONFIG_HOME": "/c", "XDG_DATA_HOME": "/d", "XDG_CACHE_HOME": "/k", "XDG_STATE_HOME": "/s",
	})); err != nil {
		t.Errorf("linux with full XDG and no home: %v", err)
	}
}

func TestFor(t *testing.T) {
	d, err := For("com.example.test")
	if err != nil {
		t.Skip("no home:", err)
	}
	for _, p := range []string{d.Config, d.Data, d.Cache, d.Logs} {
		if !filepath.IsAbs(p) {
			t.Errorf("%q is not absolute", p)
		}
	}
	if c, _ := Config("com.example.test"); c != d.Config {
		t.Errorf("Config = %q, want %q", c, d.Config)
	}
}
