//go:build linux || darwin

package gl

import (
	"bytes"
	"fmt"
	"runtime"
)

// forceCompileSource, when set (tests), replaces the sources
// compileProgram is given, to exercise the driver rejecting a shader.
var forceCompileSource func(vert, frag string) (string, string)

// compileProgram compiles the vertex and fragment shader sources and links
// them into a program, returning it (the shaders are deleted). When a
// stage does not compile or the program does not link, nothing is kept
// and the error's first line names the stage; the driver's info log
// follows on the next lines. It must run on the thread f's context is
// current on.
func compileProgram(f *glFuncs, vert, frag string) (prog uint32, err error) {
	if forceCompileSource != nil {
		vert, frag = forceCompileSource(vert, frag)
	}
	vs, err := compileShader(f, glVERTEX_SHADER, "vertex", vert)
	if err != nil {
		return 0, err
	}
	defer f.DeleteShader(vs)
	fs, err := compileShader(f, glFRAGMENT_SHADER, "fragment", frag)
	if err != nil {
		return 0, err
	}
	defer f.DeleteShader(fs)
	prog = f.CreateProgram()
	if prog == 0 {
		return 0, fmt.Errorf("gl: glCreateProgram returned 0 (glGetError %#x)", f.GetError())
	}
	f.AttachShader(prog, vs)
	f.AttachShader(prog, fs)
	f.LinkProgram(prog)
	f.DetachShader(prog, vs)
	f.DetachShader(prog, fs)
	var ok int32
	f.GetProgramiv(prog, glLINK_STATUS, &ok)
	if ok == glFALSE {
		var n int32
		f.GetProgramiv(prog, glINFO_LOG_LENGTH, &n)
		log := infoLog(n, func(size int32, length *int32, buf *byte) { f.GetProgramInfoLog(prog, size, length, buf) })
		f.DeleteProgram(prog)
		return 0, fmt.Errorf("gl: the shader program did not link:\n%s", log)
	}
	return prog, nil
}

// compileShader compiles one stage (typ VERTEX_SHADER or FRAGMENT_SHADER,
// stage its name in errors).
func compileShader(f *glFuncs, typ uint32, stage, src string) (uint32, error) {
	s := f.CreateShader(typ)
	if s == 0 {
		return 0, fmt.Errorf("gl: glCreateShader(%s) returned 0 (glGetError %#x)", stage, f.GetError())
	}
	// One NUL-terminated string with its length: the pointer array and
	// the bytes stay alive until glShaderSource has copied them.
	csrc := append([]byte(src), 0)
	ptr := &csrc[0]
	n := int32(len(src))
	f.ShaderSource(s, 1, &ptr, &n)
	runtime.KeepAlive(csrc)
	f.CompileShader(s)
	var ok int32
	f.GetShaderiv(s, glCOMPILE_STATUS, &ok)
	if ok == glFALSE {
		var size int32
		f.GetShaderiv(s, glINFO_LOG_LENGTH, &size)
		log := infoLog(size, func(size int32, length *int32, buf *byte) { f.GetShaderInfoLog(s, size, length, buf) })
		f.DeleteShader(s)
		return 0, fmt.Errorf("gl: the %s shader did not compile:\n%s", stage, log)
	}
	return s, nil
}

// infoLog reads an info log of size bytes (INFO_LOG_LENGTH, with its NUL)
// through get.
func infoLog(size int32, get func(size int32, length *int32, buf *byte)) string {
	if size <= 1 {
		return "(the driver gave no info log)"
	}
	buf := make([]byte, size)
	var n int32
	get(size, &n, &buf[0])
	if n < 0 || n > size {
		n = size
	}
	log := bytes.TrimRight(buf[:n], "\x00 \t\r\n")
	if len(log) == 0 {
		return "(the driver gave no info log)"
	}
	return string(log)
}
