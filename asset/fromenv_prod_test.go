//go:build !tango_dev

package asset

import "testing"

// Production builds ignore TANGO_DEV_URL: another process setting it must
// not make the app load someone else's pages.
func TestFromEnvProd(t *testing.T) {
	for _, v := range []string{"", "http://localhost:5173", "http://evil.example"} {
		t.Setenv(DevURLEnv, v)
		if _, ok := FromEnv(testFS).(*fileHandler); !ok {
			t.Errorf("%s=%q: want FS handler in a production build", DevURLEnv, v)
		}
	}
}
