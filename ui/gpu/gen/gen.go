// Package gen holds the shader sources generated from ui/gpu/shaders. The
// files are committed so that normal builds need no shader compiler; run
// go generate ./ui/gpu/gen/ after changing a shader
// (ui/gpu/shaders TestGeneratedUpToDate checks this).
package gen

import _ "embed"

//go:generate go run ../../shader/cmd/tanshader -pkg ../shaders -out .

// QuadMSL is the Metal source of the quad shader (entry points vs_main and
// fs_main).
//
//go:embed quad.metal
var QuadMSL string

// QuadHLSL is the HLSL source of the quad shader (shader model 5.0, also
// SM4-compatible; entry points vs_main and ps_main, compiled as vs_5_0 and
// ps_5_0, or vs_4_0 and ps_4_0 on a feature level 10_x device).
//
//go:embed quad.hlsl
var QuadHLSL string
