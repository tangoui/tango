//go:build windows && !amd64 && !arm64

package windows

import (
	"errors"

	"github.com/tangoui/tango/internal/platform"
)

// Only 64-bit Windows (amd64, arm64) is supported.

var errUnsupportedArch = errors.New("tango: the Windows backend supports amd64 and arm64 only")

type unsupported struct{}

func New() platform.Backend { return unsupported{} }

func (unsupported) Init(platform.Host, platform.AppOptions) error { return errUnsupportedArch }
func (unsupported) Run() error                                    { return errUnsupportedArch }
func (unsupported) Quit()                                         {}
func (unsupported) Wake()                                         {}
func (unsupported) Step(bool) bool                                { return false }
func (unsupported) NewSurface(platform.SurfaceID, platform.SurfaceOptions) (platform.Surface, error) {
	return nil, errUnsupportedArch
}
func (unsupported) AssetOrigin() string { return assetOrigin }
