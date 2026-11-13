package env

import (
	"strconv"
	"strings"
)

// osReleaseName returns PRETTY_NAME (else NAME + VERSION) from an
// os-release file.
func osReleaseName(data string) string {
	kv := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(k, "#") {
			continue
		}
		if u, err := strconv.Unquote(v); err == nil {
			v = u
		} else {
			v = strings.Trim(v, `'"`)
		}
		kv[k] = v
	}
	if p := kv["PRETTY_NAME"]; p != "" {
		return p
	}
	return strings.TrimSpace(kv["NAME"] + " " + kv["VERSION"])
}
