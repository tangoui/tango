//go:build linux || darwin

package gl

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unsafe"

	"github.com/ebitengine/purego"
)

// glFuncs is the GL function table of one context: every GL entry point
// the backend calls, each field named after its GL function without the
// gl prefix (TexSubImage2D is glTexSubImage2D) and bound by load. GL
// types map to Go as GLenum, GLbitfield, GLuint → uint32; GLint,
// GLsizei → int32; GLboolean → uint8; GLintptr, GLsizeiptr → int; a
// buffer offset passed as a pointer → uintptr. String arguments are
// passed NUL-terminated by purego, and a Go pointer argument stays alive
// for the call.
//
// A table belongs to the context it was loaded for (on Linux libepoxy
// resolves each entry point for the current context's window system);
// call its functions only on the thread that context is current on.
type glFuncs struct {
	// State and queries
	GetString   func(name uint32) string
	GetStringi  func(name, index uint32) string
	GetIntegerv func(pname uint32, data *int32)
	GetError    func() uint32
	Enable      func(cap uint32)
	Disable     func(cap uint32)
	Viewport    func(x, y, width, height int32)
	Scissor     func(x, y, width, height int32)
	PixelStorei func(pname uint32, param int32)
	Finish      func()
	Flush       func()

	// Clearing and blending
	ClearColor             func(r, g, b, a float32)
	Clear                  func(mask uint32)
	ClearBufferfv          func(buffer uint32, drawbuffer int32, value *float32)
	BlendFunc              func(sfactor, dfactor uint32)
	BlendFuncSeparate      func(srcRGB, dstRGB, srcAlpha, dstAlpha uint32)
	BlendEquationSeparate  func(modeRGB, modeAlpha uint32)
	ColorMask              func(r, g, b, a uint8)
	ReadBuffer             func(mode uint32)
	ReadPixels             func(x, y, width, height int32, format, typ uint32, pixels unsafe.Pointer)
	BlitFramebuffer        func(srcX0, srcY0, srcX1, srcY1, dstX0, dstY0, dstX1, dstY1 int32, mask, filter uint32)
	GenFramebuffers        func(n int32, framebuffers *uint32)
	DeleteFramebuffers     func(n int32, framebuffers *uint32)
	BindFramebuffer        func(target, framebuffer uint32)
	FramebufferTexture2D   func(target, attachment, textarget, texture uint32, level int32)
	CheckFramebufferStatus func(target uint32) uint32

	// Textures and samplers
	GenTextures       func(n int32, textures *uint32)
	DeleteTextures    func(n int32, textures *uint32)
	BindTexture       func(target, texture uint32)
	ActiveTexture     func(texture uint32)
	TexImage2D        func(target uint32, level, internalformat, width, height, border int32, format, typ uint32, pixels unsafe.Pointer)
	TexSubImage2D     func(target uint32, level, xoffset, yoffset, width, height int32, format, typ uint32, pixels unsafe.Pointer)
	TexParameteri     func(target, pname uint32, param int32)
	CopyTexSubImage2D func(target uint32, level, xoffset, yoffset, x, y, width, height int32)
	GenSamplers       func(n int32, samplers *uint32)
	DeleteSamplers    func(n int32, samplers *uint32)
	BindSampler       func(unit, sampler uint32)
	SamplerParameteri func(sampler, pname uint32, param int32)

	// Buffers and vertex arrays
	GenBuffers              func(n int32, buffers *uint32)
	DeleteBuffers           func(n int32, buffers *uint32)
	BindBuffer              func(target, buffer uint32)
	BufferData              func(target uint32, size int, data unsafe.Pointer, usage uint32)
	BufferSubData           func(target uint32, offset, size int, data unsafe.Pointer)
	BindBufferBase          func(target, index, buffer uint32)
	BindBufferRange         func(target, index, buffer uint32, offset, size int)
	MapBufferRange          func(target uint32, offset, length int, access uint32) unsafe.Pointer
	UnmapBuffer             func(target uint32) uint8
	GenVertexArrays         func(n int32, arrays *uint32)
	DeleteVertexArrays      func(n int32, arrays *uint32)
	BindVertexArray         func(array uint32)
	EnableVertexAttribArray func(index uint32)
	VertexAttribPointer     func(index uint32, size int32, typ uint32, normalized uint8, stride int32, offset uintptr)
	VertexAttribIPointer    func(index uint32, size int32, typ uint32, stride int32, offset uintptr)
	VertexAttribDivisor     func(index, divisor uint32)
	DrawArrays              func(mode uint32, first, count int32)
	DrawArraysInstanced     func(mode uint32, first, count, instancecount int32)

	// Shaders and programs
	CreateShader         func(typ uint32) uint32
	ShaderSource         func(shader uint32, count int32, sources **byte, lengths *int32)
	CompileShader        func(shader uint32)
	GetShaderiv          func(shader, pname uint32, params *int32)
	GetShaderInfoLog     func(shader uint32, bufSize int32, length *int32, infoLog *byte)
	DeleteShader         func(shader uint32)
	CreateProgram        func() uint32
	AttachShader         func(program, shader uint32)
	DetachShader         func(program, shader uint32)
	LinkProgram          func(program uint32)
	GetProgramiv         func(program, pname uint32, params *int32)
	GetProgramInfoLog    func(program uint32, bufSize int32, length *int32, infoLog *byte)
	DeleteProgram        func(program uint32)
	UseProgram           func(program uint32)
	GetUniformLocation   func(program uint32, name string) int32
	Uniform1i            func(location, v0 int32)
	GetUniformBlockIndex func(program uint32, name string) uint32
	UniformBlockBinding  func(program, blockIndex, blockBinding uint32)

	// Sync objects (GLsync is a pointer)
	FenceSync      func(condition, flags uint32) uintptr
	ClientWaitSync func(sync uintptr, flags uint32, timeout uint64) uint32
	DeleteSync     func(sync uintptr)
}

// load binds every function of the table through proc, which returns the
// address of a GL function by name ("glTexSubImage2D") or 0. When any is
// missing, nothing is bound and the error lists every missing name.
func (f *glFuncs) load(proc func(name string) uintptr) error {
	v := reflect.ValueOf(f).Elem()
	t := v.Type()
	addrs := make([]uintptr, t.NumField())
	var missing []string
	for i := range t.NumField() {
		name := "gl" + t.Field(i).Name
		if addrs[i] = proc(name); addrs[i] == 0 {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("gl: %d GL functions not found: %s", len(missing), strings.Join(missing, ", "))
	}
	for i := range t.NumField() {
		purego.RegisterFunc(v.Field(i).Addr().Interface(), addrs[i])
	}
	return nil
}

// version returns the context's GL version (GL_MAJOR_VERSION and
// GL_MINOR_VERSION, OpenGL 3.0 and later).
func (f *glFuncs) version() (major, minor int) {
	var ma, mi int32
	f.GetIntegerv(glMAJOR_VERSION, &ma)
	f.GetIntegerv(glMINOR_VERSION, &mi)
	return int(ma), int(mi)
}

// parseVersion returns the version a GL_VERSION string starts with
// ("4.5 (Core Profile) Mesa 22.3.6"; an "OpenGL ES" prefix is skipped).
func parseVersion(s string) (major, minor int, ok bool) {
	s = strings.TrimPrefix(s, "OpenGL ES ")
	num, _, _ := strings.Cut(s, " ")
	a, rest, ok := strings.Cut(num, ".")
	if !ok {
		return 0, 0, false
	}
	b, _, _ := strings.Cut(rest, ".")
	major, err1 := strconv.Atoi(a)
	minor, err2 := strconv.Atoi(b)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return major, minor, true
}

// bindLib binds each function pointer in fns (symbol name → pointer to a
// func variable) to the symbol of lib (libName in errors). A missing
// symbol binds nothing and returns an error naming every missing one.
func bindLib(lib uintptr, libName string, fns map[string]any) error {
	addrs := make(map[string]uintptr, len(fns))
	var missing []string
	for name := range fns {
		a, err := purego.Dlsym(lib, name)
		if err != nil || a == 0 {
			missing = append(missing, name)
			continue
		}
		addrs[name] = a
	}
	if len(missing) > 0 {
		return fmt.Errorf("gl: %s lacks %s", libName, strings.Join(missing, ", "))
	}
	for name, fp := range fns {
		purego.RegisterFunc(fp, addrs[name])
	}
	return nil
}

// GL enums the backend uses: the GL name with gl in place of its GL_
// prefix (glMAX_TEXTURE_SIZE is GL_MAX_TEXTURE_SIZE), unexported.
const (
	glFALSE    = 0
	glTRUE     = 1
	glNO_ERROR = 0

	glINVALID_ENUM                  = 0x0500
	glINVALID_VALUE                 = 0x0501
	glINVALID_OPERATION             = 0x0502
	glOUT_OF_MEMORY                 = 0x0505
	glINVALID_FRAMEBUFFER_OPERATION = 0x0506
	glINVALID_INDEX                 = 0xFFFFFFFF

	// glGetGraphicsResetStatus results (OpenGL 4.5, GL_KHR_robustness,
	// GL_ARB_robustness); GL_NO_ERROR when no reset happened.
	glGUILTY_CONTEXT_RESET   = 0x8253
	glINNOCENT_CONTEXT_RESET = 0x8254
	glUNKNOWN_CONTEXT_RESET  = 0x8255

	// Queries
	glVENDOR                          = 0x1F00
	glRENDERER                        = 0x1F01
	glVERSION                         = 0x1F02
	glEXTENSIONS                      = 0x1F03
	glSHADING_LANGUAGE_VERSION        = 0x8B8C
	glMAJOR_VERSION                   = 0x821B
	glMINOR_VERSION                   = 0x821C
	glNUM_EXTENSIONS                  = 0x821D
	glMAX_TEXTURE_SIZE                = 0x0D33
	glMAX_TEXTURE_IMAGE_UNITS         = 0x8872
	glMAX_UNIFORM_BLOCK_SIZE          = 0x8A30
	glUNIFORM_BUFFER_OFFSET_ALIGNMENT = 0x8A34
	glVIEWPORT                        = 0x0BA2
	glDRAW_FRAMEBUFFER_BINDING        = 0x8CA6
	glREAD_FRAMEBUFFER_BINDING        = 0x8CAA

	// Capabilities
	glBLEND            = 0x0BE2
	glSCISSOR_TEST     = 0x0C11
	glDEPTH_TEST       = 0x0B71
	glSTENCIL_TEST     = 0x0B90
	glCULL_FACE        = 0x0B44
	glDITHER           = 0x0BD0
	glFRAMEBUFFER_SRGB = 0x8DB9

	// State a context shared with a toolkit may be left in
	glRASTERIZER_DISCARD       = 0x8C89
	glSAMPLE_ALPHA_TO_COVERAGE = 0x809E
	glFRONT_AND_BACK           = 0x0408
	glLINE                     = 0x1B01
	glFILL                     = 0x1B02

	// Blending and clearing
	glZERO                = 0
	glONE                 = 1
	glSRC_ALPHA           = 0x0302
	glONE_MINUS_SRC_ALPHA = 0x0303
	glFUNC_ADD            = 0x8006
	glCOLOR_BUFFER_BIT    = 0x4000
	glCOLOR               = 0x1800

	// Data types and pixel formats
	glUNSIGNED_BYTE  = 0x1401
	glUNSIGNED_SHORT = 0x1403
	glINT            = 0x1404
	glUNSIGNED_INT   = 0x1405
	glFLOAT          = 0x1406
	glHALF_FLOAT     = 0x140B
	glRED            = 0x1903
	glRG             = 0x8227
	glRGBA           = 0x1908
	glR8             = 0x8229
	glR16            = 0x822A
	glRGBA8          = 0x8058
	glRGBA16         = 0x805B

	// Textures
	glTEXTURE_2D         = 0x0DE1
	glTEXTURE0           = 0x84C0
	glTEXTURE_MAG_FILTER = 0x2800
	glTEXTURE_MIN_FILTER = 0x2801
	glTEXTURE_WRAP_S     = 0x2802
	glTEXTURE_WRAP_T     = 0x2803
	glTEXTURE_BASE_LEVEL = 0x813C
	glTEXTURE_MAX_LEVEL  = 0x813D
	glNEAREST            = 0x2600
	glLINEAR             = 0x2601
	glCLAMP_TO_EDGE      = 0x812F

	// Pixel storage
	glUNPACK_ROW_LENGTH  = 0x0CF2
	glUNPACK_SKIP_ROWS   = 0x0CF3
	glUNPACK_SKIP_PIXELS = 0x0CF4
	glUNPACK_ALIGNMENT   = 0x0CF5
	glPACK_ROW_LENGTH    = 0x0D02
	glPACK_SKIP_ROWS     = 0x0D03
	glPACK_SKIP_PIXELS   = 0x0D04
	glPACK_ALIGNMENT     = 0x0D05

	// Framebuffers
	glFRAMEBUFFER          = 0x8D40
	glREAD_FRAMEBUFFER     = 0x8CA8
	glDRAW_FRAMEBUFFER     = 0x8CA9
	glCOLOR_ATTACHMENT0    = 0x8CE0
	glFRAMEBUFFER_COMPLETE = 0x8CD5
	glBACK                 = 0x0405

	// Buffers
	glARRAY_BUFFER              = 0x8892
	glPIXEL_PACK_BUFFER         = 0x88EB
	glPIXEL_UNPACK_BUFFER       = 0x88EC
	glUNIFORM_BUFFER            = 0x8A11
	glSTREAM_DRAW               = 0x88E0
	glSTATIC_DRAW               = 0x88E4
	glDYNAMIC_DRAW              = 0x88E8
	glMAP_WRITE_BIT             = 0x0002
	glMAP_INVALIDATE_RANGE_BIT  = 0x0004
	glMAP_INVALIDATE_BUFFER_BIT = 0x0008
	glMAP_UNSYNCHRONIZED_BIT    = 0x0020

	// Drawing
	glTRIANGLES      = 0x0004
	glTRIANGLE_STRIP = 0x0005

	// Shaders and programs
	glFRAGMENT_SHADER = 0x8B30
	glVERTEX_SHADER   = 0x8B31
	glCOMPILE_STATUS  = 0x8B81
	glLINK_STATUS     = 0x8B82
	glINFO_LOG_LENGTH = 0x8B84
	glACTIVE_UNIFORMS = 0x8B86

	// Sync objects
	glSYNC_GPU_COMMANDS_COMPLETE = 0x9117
	glSYNC_FLUSH_COMMANDS_BIT    = 0x0001
	glALREADY_SIGNALED           = 0x911A
	glTIMEOUT_EXPIRED            = 0x911B
	glCONDITION_SATISFIED        = 0x911C
	glWAIT_FAILED                = 0x911D
)
