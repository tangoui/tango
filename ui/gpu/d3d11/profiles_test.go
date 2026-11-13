package d3d11

import (
	"testing"

	"github.com/tangoui/tango/ui/gpu/gen"
	"github.com/tangoui/tango/ui/shader/hlsl"
)

// TestProfilesMatchGenerator keeps ui/gpu/gen's hand-written entry points
// and compile targets (used by the device, which must not link the
// generator) equal to ui/shader/hlsl's, which the generated source and
// its header follow. Only this test imports ui/shader/hlsl.
func TestProfilesMatchGenerator(t *testing.T) {
	for _, c := range []struct{ name, gen, hlsl string }{
		{"vertex entry", gen.QuadVSEntry, hlsl.VertexEntry},
		{"pixel entry", gen.QuadPSEntry, hlsl.FragmentEntry},
		{"SM5 vertex target", gen.QuadVSTarget, hlsl.VertexTarget},
		{"SM5 pixel target", gen.QuadPSTarget, hlsl.FragmentTarget},
		{"SM4 vertex target", gen.QuadVSTargetSM4, hlsl.VertexTargetSM4},
		{"SM4 pixel target", gen.QuadPSTargetSM4, hlsl.FragmentTargetSM4},
	} {
		if c.gen != c.hlsl {
			t.Errorf("%s: gen %q, hlsl %q", c.name, c.gen, c.hlsl)
		}
	}
	if vs, ps := shaderProfiles(featureLevel11_0); vs != hlsl.VertexTarget || ps != hlsl.FragmentTarget {
		t.Errorf("11_0 profiles %s %s", vs, ps)
	}
	if vs, ps := shaderProfiles(featureLevel10_0); vs != hlsl.VertexTargetSM4 || ps != hlsl.FragmentTargetSM4 {
		t.Errorf("10_0 profiles %s %s", vs, ps)
	}
}
