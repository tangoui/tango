package platform

import "testing"

func TestDefaultIMEConnectionUsesAppName(t *testing.T) {
	for _, tc := range []struct {
		opts AppOptions
		want string
	}{
		{AppOptions{Name: "TestIME"}, "TestIME_Connection"},
		{AppOptions{Name: "TestIME", ID: "com.example.inputmethod"}, "com.example.inputmethod_Connection"},
	} {
		if got := tc.opts.DefaultIMEConnection(); got != tc.want {
			t.Errorf("default connection for %+v = %q, want %q", tc.opts, got, tc.want)
		}
	}
}
