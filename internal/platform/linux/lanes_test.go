package linux

import (
	"reflect"
	"testing"
)

func TestLinuxLaneCapsGating(t *testing.T) {
	cases := []struct {
		name, setting, global string
		hasBody, hasStream    bool
		want                  []string
	}{
		{name: "both", setting: "1", hasBody: true, hasStream: true, want: []string{"down:poll", "up:post"}},
		{name: "missing body", setting: "1", hasStream: true, want: []string{"down:poll"}},
		{name: "missing stream", setting: "1", hasBody: true, want: []string{"up:post"}},
		{name: "missing both", setting: "1"},
		{name: "default", hasBody: true, hasStream: true},
		{name: "unknown", setting: "true", hasBody: true, hasStream: true},
		{name: "all", setting: " ALL ", hasBody: true, hasStream: true, want: []string{"down:poll", "up:post"}},
		{name: "up only", setting: "up", hasBody: true, hasStream: true, want: []string{"up:post"}},
		{name: "up unavailable", setting: "up", hasStream: true},
		{name: "down only", setting: "down", hasBody: true, hasStream: true, want: []string{"down:poll"}},
		{name: "down unavailable", setting: "down", hasBody: true},
		{name: "zero", setting: "0", hasBody: true, hasStream: true},
		{name: "false", setting: "false", hasBody: true, hasStream: true},
		{name: "off", setting: "off", hasBody: true, hasStream: true},
		{name: "kill switch", setting: "1", global: "0", hasBody: true, hasStream: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := map[string]string{"TANGO_IPC_LANES_LINUX": c.setting, "TANGO_IPC_LANES": c.global}
			got := laneCapsFromEnv(func(k string) string { return env[k] }, c.hasBody, c.hasStream)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("caps = %v, want %v", got, c.want)
			}
		})
	}
}
