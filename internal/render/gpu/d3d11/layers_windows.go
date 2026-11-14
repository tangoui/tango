//go:build windows && (amd64 || arm64)

package d3d11

import (
	"image"

	"github.com/tangoui/tango/internal/render/gpu/internal/devkit"
)

// newLayerPool returns the pool of layer render targets (layerPool,
// layers.go) of the device: R8G8B8A8_UNORM textures with a render-target
// and a shader-resource view.
func (d *Device) newLayerPool() *layerPool[texture] {
	return &layerPool[texture]{devkit.LayerPool[texture]{
		NewTex: func(size image.Point) (texture, error) {
			return newTexture(d.dev, size, layerSpec, nil, 0)
		},
		Free: func(t texture) { t.release() },
	}}
}

// layerTextures returns the number of layer textures the device holds
// (tests).
func (d *Device) layerTextures() int { return d.layers.Count() }
