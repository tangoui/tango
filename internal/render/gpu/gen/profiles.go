package gen

// Entry points and compile targets of QuadHLSL, for the Direct3D 11
// device. They repeat internal/render/shader/hlsl's VertexEntry, FragmentEntry,
// VertexTarget, FragmentTarget, VertexTargetSM4 and FragmentTargetSM4
// (hand-written here so the device does not link the generator;
// internal/render/gpu/d3d11's TestProfilesMatchGenerator keeps them equal).
const (
	QuadVSEntry = "vs_main"
	QuadPSEntry = "ps_main"

	// Shader model 5.0, for feature level 11_0 and above.
	QuadVSTarget = "vs_5_0"
	QuadPSTarget = "ps_5_0"
	// Shader model 4.0, for feature levels 10_0 and 10_1.
	QuadVSTargetSM4 = "vs_4_0"
	QuadPSTargetSM4 = "ps_4_0"
)
