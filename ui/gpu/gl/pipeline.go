//go:build linux || darwin

package gl

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"unsafe"

	"github.com/tangoui/tango/ui/gpu/gen"
	"github.com/tangoui/tango/ui/gpu/shaders"
)

// pipeline is the fixed state the quad shader draws with: the program,
// the vertex array object describing the per-instance buffer, one sampler
// object per texture unit and the FrameU uniform buffer. Every non-zero
// name is an object of the device's context the pipeline owns; every
// method runs on that context's thread.
type pipeline struct {
	prog     uint32
	vao      uint32
	inst     uint32 // GL_ARRAY_BUFFER: the frame's instances
	instCap  int    // bytes of inst's storage
	samplers []uint32
	frameU   uint32 // GL_UNIFORM_BUFFER holding shaders.Frame
	frameUAt uint32 // its uniform buffer binding point
}

// minInstBuffer is the smallest instance buffer allocated (455 instances).
const minInstBuffer = 64 << 10

// quadBinding is a sampler or uniform block of the quad shader and the
// binding index the generator gave it.
type quadBinding struct {
	name    string
	binding int
}

// The generator (ui/shader/glsl) declares every sampler as
// "uniform sampler2D <name>; // binding N, filter F" and every uniform
// block as "layout(std140) uniform <Name>_ub { ... }; // binding N", one
// per line at the start of the line; gen.QuadTextures and the uniform
// layout carry the binding but not the GLSL name, so the device reads the
// names from the generated sources (TestQuadBindings pins the result).
var (
	samplerDecl = regexp.MustCompile(`(?m)^uniform sampler2D (\w+); // binding (\d+)`)
	blockDecl   = regexp.MustCompile(`(?ms)^layout\(std140\) uniform (\w+) \{.*?^\}; // binding (\d+)`)
)

// quadBindings returns the samplers and uniform blocks declared by the
// quad shader's vertex and fragment sources, each once, by binding.
func quadBindings(vert, frag string) (samplers, blocks []quadBinding, err error) {
	collect := func(re *regexp.Regexp) ([]quadBinding, error) {
		var out []quadBinding
		for _, src := range []string{vert, frag} {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				n, err := strconv.Atoi(m[2])
				if err != nil {
					return nil, fmt.Errorf("gl: quad shader: binding %q of %s", m[2], m[1])
				}
				b := quadBinding{name: m[1], binding: n}
				if i := slices.IndexFunc(out, func(o quadBinding) bool { return o.name == b.name }); i >= 0 {
					if out[i] != b {
						return nil, fmt.Errorf("gl: quad shader: %s has binding %d and %d", b.name, out[i].binding, b.binding)
					}
					continue
				}
				out = append(out, b)
			}
		}
		slices.SortFunc(out, func(a, b quadBinding) int { return a.binding - b.binding })
		return out, nil
	}
	if samplers, err = collect(samplerDecl); err != nil {
		return nil, nil, err
	}
	if blocks, err = collect(blockDecl); err != nil {
		return nil, nil, err
	}
	return samplers, blocks, nil
}

// frameUBlock is the uniform block holding shaders.FrameU, and frameUSize
// its buffer's size: the Go struct's size rounded up to 16 bytes (std140
// rounds a block up to a vec4; ui/shader/glsl checks the Go layout
// matches std140 member by member).
const (
	frameUBlock = "FrameU_ub"
	frameUSize  = (int(unsafe.Sizeof(shaders.Frame{})) + 15) &^ 15
)

// newPipeline compiles the quad shader and creates the pipeline's
// objects on f's context. On error everything created is deleted.
func newPipeline(f *glFuncs) (p *pipeline, err error) {
	samplers, blocks, err := quadBindings(gen.QuadGLSLVertex, gen.QuadGLSLFragment)
	if err != nil {
		return nil, err
	}
	if len(samplers) != len(gen.QuadTextures) {
		return nil, fmt.Errorf("gl: the quad shader declares %d samplers, gen.QuadTextures %d", len(samplers), len(gen.QuadTextures))
	}
	if len(blocks) != 1 || blocks[0].name != frameUBlock {
		return nil, fmt.Errorf("gl: the quad shader declares uniform blocks %v, want only %s", blocks, frameUBlock)
	}
	p = &pipeline{}
	defer func() {
		if err != nil {
			p.release(f)
			p = nil
		}
	}()
	if p.prog, err = compileProgram(f, gen.QuadGLSLVertex, gen.QuadGLSLFragment); err != nil {
		return nil, err
	}
	f.UseProgram(p.prog)
	// Texture unit N for the sampler of binding N. A location of -1 is a
	// sampler the linker dropped as unused: nothing to set.
	for _, s := range samplers {
		if loc := f.GetUniformLocation(p.prog, s.name); loc >= 0 {
			f.Uniform1i(loc, int32(s.binding))
		}
	}
	p.frameUAt = uint32(blocks[0].binding)
	if idx := f.GetUniformBlockIndex(p.prog, frameUBlock); idx != glINVALID_INDEX {
		f.UniformBlockBinding(p.prog, idx, p.frameUAt)
	}
	f.GenBuffers(1, &p.frameU)
	f.BindBuffer(glUNIFORM_BUFFER, p.frameU)
	f.BufferData(glUNIFORM_BUFFER, frameUSize, nil, glDYNAMIC_DRAW)

	// One clamp-to-edge sampler object per texture unit, with the filter
	// the shader generator chose for it (no mipmaps).
	p.samplers = make([]uint32, len(gen.QuadTextures))
	for i, td := range gen.QuadTextures {
		if td.Binding != i {
			return nil, fmt.Errorf("gl: quad texture %d has binding %d", i, td.Binding)
		}
		filter, err := samplerFilter(td.Filter)
		if err != nil {
			return nil, err
		}
		f.GenSamplers(1, &p.samplers[i])
		s := p.samplers[i]
		f.SamplerParameteri(s, glTEXTURE_MIN_FILTER, filter)
		f.SamplerParameteri(s, glTEXTURE_MAG_FILTER, filter)
		f.SamplerParameteri(s, glTEXTURE_WRAP_S, glCLAMP_TO_EDGE)
		f.SamplerParameteri(s, glTEXTURE_WRAP_T, glCLAMP_TO_EDGE)
	}

	// The vertex array: every attribute of shaders.Instance advances once
	// per instance (divisor 1). Its pointers are set per batch (bindAt).
	f.GenVertexArrays(1, &p.vao)
	f.GenBuffers(1, &p.inst)
	f.BindVertexArray(p.vao)
	f.BindBuffer(glARRAY_BUFFER, p.inst)
	for _, at := range gen.QuadInstanceAttrs {
		if _, _, err := attribType(at.Kind, at.N); err != nil {
			return nil, fmt.Errorf("%w (attribute %d)", err, at.Location)
		}
		if at.Offset+4*at.N > gen.QuadInstanceStride {
			return nil, fmt.Errorf("gl: attribute %d at offset %d overflows the %d-byte instance", at.Location, at.Offset, gen.QuadInstanceStride)
		}
		f.EnableVertexAttribArray(uint32(at.Location))
		f.VertexAttribDivisor(uint32(at.Location), 1)
	}
	f.BindVertexArray(0)
	return p, nil
}

// attribType returns the GL component type of a vertex attribute of n
// components of kind ("f32", "i32", "u32"), as gen.QuadInstanceAttrs
// describes them, and whether it is an integer attribute
// (glVertexAttribIPointer).
func attribType(kind string, n int) (typ uint32, integer bool, err error) {
	if n < 1 || n > 4 {
		return 0, false, fmt.Errorf("gl: no attribute of %d × %s", n, kind)
	}
	switch kind {
	case "f32":
		return glFLOAT, false, nil
	case "i32":
		return glINT, true, nil
	case "u32":
		return glUNSIGNED_INT, true, nil
	}
	return 0, false, fmt.Errorf("gl: no attribute of %d × %s", n, kind)
}

// samplerFilter returns the GL filter of a gen.TextureDesc filter name.
func samplerFilter(name string) (int32, error) {
	switch name {
	case "nearest":
		return glNEAREST, nil
	case "linear":
		return glLINEAR, nil
	}
	return 0, fmt.Errorf("gl: unknown sampler filter %q", name)
}

// bind sets the pipeline's state: program, vertex array, uniform buffer,
// samplers, premultiplied "over" blending and the fixed-function state
// the quad shader assumes (scissor on; depth, stencil, culling,
// dithering and sRGB conversion off). It is set in full every frame: a
// presenter's context is shared with the toolkit, which may change it.
func (p *pipeline) bind(f *glFuncs) {
	f.UseProgram(p.prog)
	f.BindVertexArray(p.vao)
	f.BindBuffer(glARRAY_BUFFER, p.inst)
	f.BindBufferBase(glUNIFORM_BUFFER, p.frameUAt, p.frameU)
	for i, s := range p.samplers {
		f.BindSampler(uint32(i), s)
	}
	f.Enable(glBLEND)
	f.BlendEquationSeparate(glFUNC_ADD, glFUNC_ADD)
	f.BlendFunc(glONE, glONE_MINUS_SRC_ALPHA)
	f.Disable(glDEPTH_TEST)
	f.Disable(glSTENCIL_TEST)
	f.Disable(glCULL_FACE)
	f.Disable(glDITHER)
	f.Disable(glFRAMEBUFFER_SRGB)
	f.ColorMask(1, 1, 1, 1)
	f.Enable(glSCISSOR_TEST)
}

// setFrameU writes the FrameU uniform buffer. GL orders the write after
// the draws issued before it: they keep the old values.
func (p *pipeline) setFrameU(f *glFuncs, fu shaders.Frame) {
	var buf [frameUSize]byte
	*(*shaders.Frame)(unsafe.Pointer(&buf[0])) = fu
	f.BindBuffer(glUNIFORM_BUFFER, p.frameU)
	f.BufferSubData(glUNIFORM_BUFFER, 0, frameUSize, unsafe.Pointer(&buf[0]))
}

// writeInstances writes the instances of every batch, back to back from
// byte 0, into the instance buffer (bound to GL_ARRAY_BUFFER by bind).
// The buffer is orphaned first (glBufferData with no data), so the driver
// gives it fresh storage instead of waiting for the previous frame's
// draws; it grows to max(n, 2·cap, 64 KiB) bytes when too small. n is the
// total instance count (> 0).
func (p *pipeline) writeInstances(f *glFuncs, batches [][]shaders.Instance, n int) {
	size := n * gen.QuadInstanceStride
	if size > p.instCap {
		p.instCap = max(size, 2*p.instCap, minInstBuffer)
	}
	f.BufferData(glARRAY_BUFFER, p.instCap, nil, glSTREAM_DRAW)
	off := 0
	for _, ins := range batches {
		if len(ins) == 0 {
			continue
		}
		b := len(ins) * gen.QuadInstanceStride
		f.BufferSubData(glARRAY_BUFFER, off, b, unsafe.Pointer(&ins[0]))
		off += b
	}
}

// bindAt points the vertex array's attributes at the instance starting at
// byte off of the instance buffer (GL 3.3 has no base instance for
// glDrawArraysInstanced).
func (p *pipeline) bindAt(f *glFuncs, off int) {
	for _, at := range gen.QuadInstanceAttrs {
		typ, integer, _ := attribType(at.Kind, at.N) // checked by newPipeline
		ptr := uintptr(off + at.Offset)
		if integer {
			f.VertexAttribIPointer(uint32(at.Location), int32(at.N), typ, gen.QuadInstanceStride, ptr)
		} else {
			f.VertexAttribPointer(uint32(at.Location), int32(at.N), typ, glFALSE, gen.QuadInstanceStride, ptr)
		}
	}
}

// release deletes every object the pipeline holds. Safe on a partially
// built pipeline.
func (p *pipeline) release(f *glFuncs) {
	if p == nil {
		return
	}
	for i := range p.samplers {
		if p.samplers[i] != 0 {
			f.DeleteSamplers(1, &p.samplers[i])
			p.samplers[i] = 0
		}
	}
	for _, b := range []*uint32{&p.inst, &p.frameU} {
		if *b != 0 {
			f.DeleteBuffers(1, b)
			*b = 0
		}
	}
	if p.vao != 0 {
		f.DeleteVertexArrays(1, &p.vao)
		p.vao = 0
	}
	if p.prog != 0 {
		f.DeleteProgram(p.prog)
		p.prog = 0
	}
	p.instCap = 0
}
