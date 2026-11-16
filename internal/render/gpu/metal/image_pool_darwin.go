//go:build darwin

package metal

import (
	"image"
	"runtime"
)

// Small recurring images (such as minimap strips whose widths change while
// typing) can reuse an exact-size texture from an earlier frame. This cap is
// nominal RGBA bytes; Metal object/driver overhead is additional. It is separate
// from the larger, transient same-frame handoff limit.
const (
	imageSpareCount      = 16
	imageSpareBytes      = 1 << 20
	imageSpareIdleFrames = 120
)

type imageSpare struct {
	imageCandidate
	frame uint64
}

// imageTexturePool owns only detached textures. The insertion order is oldest
// first, so both eviction and expiry release the oldest spare first. It is
// accessed under Device.mu, after earlier frame/readback submissions have
// completed. Presents may remain in flight: they only read the target.
type imageTexturePool struct {
	items [imageSpareCount]imageSpare
	n     int
	bytes int
	frame uint64
}

func (p *imageTexturePool) remove(i int) imageCandidate {
	c := p.items[i].imageCandidate
	p.n--
	copy(p.items[i:p.n], p.items[i+1:p.n+1])
	p.items[p.n] = imageSpare{}
	p.bytes -= 4 * c.size.X * c.size.Y
	return c
}

func (p *imageTexturePool) clear() {
	for p.n > 0 {
		release(p.remove(p.n - 1).tex)
	}
}

func (p *imageTexturePool) beginFrame(noUploads bool) {
	p.frame++
	if noUploads {
		p.clear()
		return
	}
	for p.n > 0 && p.frame-p.items[0].frame > imageSpareIdleFrames {
		release(p.remove(0).tex)
	}
}

func (p *imageTexturePool) take(size image.Point) id {
	for i := p.n - 1; i >= 0; i-- {
		if p.items[i].size == size {
			return p.remove(i).tex
		}
	}
	return 0
}

// put transfers ownership even when the texture is too large to keep.
func (p *imageTexturePool) put(c imageCandidate) {
	n := 4 * c.size.X * c.size.Y
	if n <= 0 || n > imageSpareBytes {
		release(c.tex)
		return
	}
	for p.n == len(p.items) || p.bytes+n > imageSpareBytes {
		release(p.remove(0).tex)
	}
	p.items[p.n] = imageSpare{imageCandidate: c, frame: p.frame}
	p.n++
	p.bytes += n
}

// TrimImages drops unused image textures immediately on memory pressure.
// Necessary live textures keep their usual frame-local handoff, but the next
// frame must not refill the spare pool from the renderer's pending frees.
func (d *Device) TrimImages() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer autoreleasePool()()
	d.imagePool.clear()
	d.imageTrimmed = true
}
