//go:build darwin && amd64

package metal

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// On amd64 (System V) structs larger than 16 bytes (MTLOrigin, MTLSize,
// MTLScissorRect) are copied by value onto the stack, which purego does for
// struct arguments.
var (
	msgScissorV      func(o id, s objc.SEL, r mtlScissorRect)
	msgBufToTextureV func(o id, s objc.SEL, buf id, off, bpr, bpi uint64, size mtlSize, tex id, slice, level uint64, origin mtlOrigin)
	msgTexToBufferV  func(o id, s objc.SEL, tex id, slice, level uint64, origin mtlOrigin, size mtlSize, buf id, off, bpr, bpi uint64)
	msgTexToTextureV func(o id, s objc.SEL, src id, slice, level uint64, origin mtlOrigin, size mtlSize, dst id, dslice, dlevel uint64, dorigin mtlOrigin)
)

func registerBigArgs(send uintptr) {
	purego.RegisterFunc(&msgScissorV, send)
	purego.RegisterFunc(&msgBufToTextureV, send)
	purego.RegisterFunc(&msgTexToBufferV, send)
	purego.RegisterFunc(&msgTexToTextureV, send)
}

func setScissorRect(enc id, r mtlScissorRect) {
	msgScissorV(enc, sel("setScissorRect:"), r)
}

func copyBufferToTexture(enc, buf id, off, bpr, bpi uint64, size mtlSize, tex id, origin mtlOrigin) {
	msgBufToTextureV(enc, sel("copyFromBuffer:sourceOffset:sourceBytesPerRow:sourceBytesPerImage:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
		buf, off, bpr, bpi, size, tex, 0, 0, origin)
}

func copyTextureToBuffer(enc, tex id, origin mtlOrigin, size mtlSize, buf id, off, bpr, bpi uint64) {
	msgTexToBufferV(enc, sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toBuffer:destinationOffset:destinationBytesPerRow:destinationBytesPerImage:"),
		tex, 0, 0, origin, size, buf, off, bpr, bpi)
}

func copyTextureToTexture(enc, src id, origin mtlOrigin, size mtlSize, dst id, dorigin mtlOrigin) {
	msgTexToTextureV(enc, sel("copyFromTexture:sourceSlice:sourceLevel:sourceOrigin:sourceSize:toTexture:destinationSlice:destinationLevel:destinationOrigin:"),
		src, 0, 0, origin, size, dst, 0, 0, dorigin)
}
