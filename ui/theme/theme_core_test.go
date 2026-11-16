package theme

import (
	"reflect"
	"testing"
)

func themeCoreComparable(t *Theme) *Theme {
	c := *t
	c.Icons = nil
	c.Components = Components{}
	c.comp = nil
	return &c
}

func TestThemeCoreParity(t *testing.T) {
	antOpts := collect([]Option{Dark(), Compact(), WithPrimary(Hex("#13c2c2"))})
	ant := AntDesign(Dark(), Compact(), WithPrimary(Hex("#13c2c2")))
	if got := antDesignCore(antOpts, false); !reflect.DeepEqual(themeCoreComparable(got), themeCoreComparable(ant)) {
		t.Fatal("Ant core tokens differ from the public constructor")
	}
	elemOpts := collect([]Option{WithPrimary(Hex("#7c3aed"))})
	elem := ElementPlus(WithPrimary(Hex("#7c3aed")))
	if got := elementPlusCore(elemOpts, false); !reflect.DeepEqual(themeCoreComparable(got), themeCoreComparable(elem)) {
		t.Fatal("Element core tokens differ from the public constructor")
	}
	sh := ShadcnOptions{Base: "slate", Radius: 8, Dark: true, Primary: Hex("#2563eb")}
	if got := shadcnCore(sh, false, Components{}); !reflect.DeepEqual(themeCoreComparable(got), themeCoreComparable(Shadcn(sh))) {
		t.Fatal("shadcn core tokens differ from the public constructor")
	}
}

func TestThemeAliasParity(t *testing.T) {
	green := Hex("#00b96b")
	got := antDesignCore(collect([]Option{Dark(), WithPrimary(Hex("#13c2c2"))}), false)
	got.adoptPrimary(green)
	want := antDesignCore(collect([]Option{Dark(), WithPrimary(green)}), false)
	if !reflect.DeepEqual(themeCoreComparable(got), themeCoreComparable(want)) {
		t.Fatal("alias reconstruction differs from a directly constructed core theme")
	}
}

func TestThemeCoreDoesNotInstallIcons(t *testing.T) {
	if antDesignCore(collect(nil), false).Icons != nil || elementPlusCore(collect(nil), false).Icons != nil {
		t.Fatal("temporary theme installed icon mapping")
	}
	if shadcnCore(ShadcnOptions{}, false, Components{}).Icons != nil {
		t.Fatal("temporary shadcn theme installed icon mapping")
	}
}
