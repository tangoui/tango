//go:build darwin && arm64

package metal

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// On arm64 the C ABI passes a composite larger than 16 bytes that is not a
// homogeneous float aggregate (MTLOrigin, MTLSize, MTLScissorRect) as a
// pointer to a caller-owned copy. purego does that for such a struct that
// lands in a register, but v0.11.1 copies one that falls on the stack (an
// argument after the eighth integer register, as in the blit copies) by
// value, which crashes the callee; so these senders take the pointers
// explicitly.
var (
	msgScissorP      func(o id, s objc.SEL, r unsafe.Pointer)
	msgBufToTextureP func(o id, s objc.SEL, buf id, off, bpr, bpi uint64, size unsafe.Pointer, tex id, slice, level uint64, origin unsafe.Pointer)
	msgTexToBufferP  func(o id, s objc.SEL, tex id, slice, level uint64, origin, size unsafe.Pointer, buf id, off, bpr, bpi uint64)
	msgTexToTextureP func(o id, s objc.SEL, src id, slice, level uint64, origin, size unsafe.Pointer, dst id, dslice, dlevel uint64, dorigin unsafe.Pointer)
)

func registerBigArgs(send uintptr) {
	purego.RegisterFunc(&msgScissorP, send)
	purego.RegisterFunc(&msgBufToTextureP, send)
	purego.RegisterFunc(&msgTexToBufferP, send)
	purego.RegisterFunc(&msgTexToTextureP, send)
}

func setScissorRect(enc id, r mtlScissorRect) {
	msgScissorP(enc, sel("setScissorRect:"), unsafe.Pointer(&r))
}

func copyBufferToTexture(enc, buf id, off, bpr, bpi uint64, size mtlSize, tex id, origin mtlOrigin) {
	msgBufToTextureP(enc, sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
		buf, off, bpr, bpi, unsafe.Pointer(&size), tex, 0, 0, unsafe.Pointer(&origin))
}

func copyTextureToBuffer(enc, tex id, origin mtlOrigin, size mtlSize, buf id, off, bpr, bpi uint64) {
	msgTexToBufferP(enc, sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:"),
		tex, 0, 0, unsafe.Pointer(&origin), unsafe.Pointer(&size), buf, off, bpr, bpi)
}

func copyTextureToTexture(enc, src id, origin mtlOrigin, size mtlSize, dst id, dorigin mtlOrigin) {
	msgTexToTextureP(enc, sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
		src, 0, 0, unsafe.Pointer(&origin), unsafe.Pointer(&size), dst, 0, 0, unsafe.Pointer(&dorigin))
}
