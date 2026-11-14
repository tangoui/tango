//go:build windows && (amd64 || arm64)

package d3d11

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/gen"
)

// TestCompileQuad compiles both entry points of the generated quad shader
// with the system's d3dcompiler_47.dll (fxc), the way the device does, for
// shader model 5.0 and 4.0.
func TestCompileQuad(t *testing.T) {
	for _, tc := range []struct{ entry, target string }{
		{gen.QuadVSEntry, gen.QuadVSTarget},
		{gen.QuadPSEntry, gen.QuadPSTarget},
		// Feature level 10_x devices (shaderProfiles).
		{gen.QuadVSEntry, gen.QuadVSTargetSM4},
		{gen.QuadPSEntry, gen.QuadPSTargetSM4},
	} {
		blob, err := compileShader(gen.QuadHLSL, tc.entry, tc.target)
		if err != nil {
			t.Fatalf("%s: %v", tc.entry, err)
		}
		// DXBC container magic.
		if len(blob) < 4 || string(blob[:4]) != "DXBC" {
			t.Errorf("%s: bytecode does not start with DXBC (%d bytes)", tc.entry, len(blob))
		}
	}
}

// TestCompileErrorIncludesLog checks that a compile failure returns the
// compiler's messages rather than just an HRESULT.
func TestCompileErrorIncludesLog(t *testing.T) {
	const bad = "float4 ps_main() : SV_Target { return undefined_name; }\n"
	_, err := compileShader(bad, "ps_main", "ps_5_0")
	if err == nil {
		t.Fatal("compiling invalid HLSL succeeded")
	}
	if !strings.Contains(err.Error(), "error X") {
		t.Fatalf("error lacks the compiler log: %v", err)
	}
	if !strings.Contains(err.Error(), shaderSourceName) {
		t.Errorf("error lacks the source name %q: %v", shaderSourceName, err)
	}
}

// TestCompileMissingEntry fails cleanly for an entry point that does not
// exist.
func TestCompileMissingEntry(t *testing.T) {
	if _, err := compileShader(gen.QuadHLSL, "no_such_entry", "ps_5_0"); err == nil {
		t.Fatal("compiling a missing entry point succeeded")
	}
}
