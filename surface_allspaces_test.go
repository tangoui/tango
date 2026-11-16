package tango

import "testing"

func TestSurfaceAllSpacesCreationAndUpdates(t *testing.T) {
	a, fb := newTestApp(t)
	s, err := a.NewSurface(SurfaceOptions{Appearance: Appearance{AllSpaces: true}})
	if err != nil {
		t.Fatal(err)
	}
	ns := fb.Surface(s.ID())
	ex, ok := ns.Extras()
	if !ok || !ex.Appearance.AllSpaces || !s.Appearance().AllSpaces {
		t.Fatal("AllSpaces was lost while creating the surface")
	}
	for _, on := range []bool{false, true} {
		if err := s.SetAppearance(Appearance{AllSpaces: on}); err != nil {
			t.Fatal(err)
		}
		ap, _ := ns.CurrentAppearance()
		if ap.AllSpaces != on || s.Appearance().AllSpaces != on {
			t.Fatalf("AllSpaces update = %v / %v, want %v", ap.AllSpaces, s.Appearance().AllSpaces, on)
		}
	}
}
