package permission_test

import (
	"errors"
	"testing"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/permission"
)

func TestPermission(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	for _, k := range permission.All {
		if st, err := permission.Check(a, k); st != permission.NotDetermined || err != nil {
			t.Fatal(k, st, err)
		}
	}
	if st, err := permission.Request(a, permission.Camera); st != permission.Granted || err != nil {
		t.Fatal(st, err)
	}
	if st, _ := permission.Check(a, permission.Camera); st != permission.Granted {
		t.Fatal(st)
	}
	fb.SetPermission(permission.Photos, permission.NotDetermined, permission.Limited)
	if st, _ := permission.Request(a, permission.Photos); st != permission.Limited {
		t.Fatal(st)
	}
	fb.SetPermission(permission.Location, permission.Denied, permission.Granted)
	if st, _ := permission.Request(a, permission.Location); st != permission.Denied {
		t.Fatal("asked again after a decision:", st)
	}
	if _, err := permission.Check(a, "telepathy"); !errors.Is(err, permission.ErrUnsupported) {
		t.Fatal(err)
	}
	if n := len(fb.PermissionRequests()); n != 3 {
		t.Fatal(n)
	}
	m, _ := faketest.RunMinimal(t, tango.Options{})
	if _, err := permission.Check(m, permission.Camera); !errors.Is(err, permission.ErrUnsupported) {
		t.Fatal(err)
	}
}
