//go:build windows && (amd64 || arm64)

package d3d11

import (
	"fmt"
	"runtime"
	"unsafe"
)

// shaderSourceName is the file name D3DCompile reports in its messages
// ("tango_quad.hlsl(12,5): error X3004: ...").
const shaderSourceName = "tango_quad.hlsl"

// compileShader compiles HLSL src at entry point entry for target (such as
// "vs_5_0") with d3dcompiler_47.dll's D3DCompile and returns the bytecode.
// A failed compile returns an error carrying the compiler's messages; a
// missing d3dcompiler_47.dll is an error wrapping ErrNoDevice (so the
// caller falls back to the CPU renderer), never a panic. With
// TANGO_D3D_DEBUG=1 the warnings of a successful compile are logged, once
// per entry point and target.
func compileShader(src, entry, target string) (blob []byte, err error) {
	if err := procD3DCompile.find(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoDevice, err)
	}
	if src == "" {
		return nil, fmt.Errorf("d3d11: compile %s (%s): empty source", entry, target)
	}
	srcBytes := []byte(src)
	name, entryZ, targetZ := cstr(shaderSourceName), cstr(entry), cstr(target)
	var code, msgs uintptr
	hr := procD3DCompile.call(
		uintptr(unsafe.Pointer(&srcBytes[0])), uintptr(len(srcBytes)),
		uintptr(unsafe.Pointer(&name[0])),
		0, // no macros
		0, // no #include handler
		uintptr(unsafe.Pointer(&entryZ[0])),
		uintptr(unsafe.Pointer(&targetZ[0])),
		compileOptimizationLevel3,
		0, // effect flags
		uintptr(unsafe.Pointer(&code)),
		uintptr(unsafe.Pointer(&msgs)),
	)
	runtime.KeepAlive(srcBytes)
	runtime.KeepAlive(name)
	runtime.KeepAlive(entryZ)
	runtime.KeepAlive(targetZ)
	log := compilerLog(blobBytes(msgs))
	release(msgs)
	if herr := hrError("D3DCompile", hr); herr != nil {
		release(code)
		if log == "" {
			log = "(no compiler output)"
		}
		return nil, fmt.Errorf("d3d11: compile %s (%s): %w\n%s", entry, target, herr, log)
	}
	blob = blobBytes(code)
	release(code)
	if len(blob) == 0 {
		return nil, fmt.Errorf("d3d11: compile %s (%s): empty bytecode", entry, target)
	}
	if log != "" && debugEnabled() {
		logOnce("compile "+entry+" "+target, "d3d11: compile %s (%s) warnings:\n%s", entry, target, log)
	}
	return blob, nil
}

// blobBytes copies the contents of an ID3DBlob; 0 gives nil.
func blobBytes(b uintptr) []byte {
	if b == 0 {
		return nil
	}
	p := comCall(b, blobGetBufferPointer)
	n := comCall(b, blobGetBufferSize)
	if p == 0 || n == 0 {
		return nil
	}
	return append([]byte(nil), unsafe.Slice((*byte)(foreign(p)), n)...)
}
