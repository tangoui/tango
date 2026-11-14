//go:build windows && (amd64 || arm64)

package d3d11

import "testing"

// TestNewDeviceFallsBackToWARP pretends the hardware adapter failed and
// expects a working WARP device, as on a machine without a GPU (virtual
// machine, remote desktop, CI).
func TestNewDeviceFallsBackToWARP(t *testing.T) {
	forceHardwareFail = true
	t.Cleanup(func() { forceHardwareFail = false })

	dev, ctx, level, err := createDevice(false)
	if err != nil {
		t.Fatalf("createDevice with hardware disabled: %v", err)
	}
	defer release(dev)
	defer release(ctx)
	if dev == 0 || ctx == 0 {
		t.Fatalf("createDevice returned device %#x, context %#x", dev, ctx)
	}
	if level < featureLevel10_0 {
		t.Errorf("feature level %#x below 10_0", level)
	}
	if got := uint32(comCall(dev, devGetFeatureLevel)); got != level {
		t.Errorf("GetFeatureLevel = %#x, createDevice reported %#x", got, level)
	}
	desc, err := adapterOf(dev)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	if !isWARP(dev) {
		t.Errorf("device runs on %q (vendor %#x, device %#x), want WARP", desc.adapterName(), desc.VendorID, desc.DeviceID)
	}
	if reason := comCall(dev, devGetDeviceRemovedReason); hrFailed(reason) {
		t.Errorf("fresh device reports removed reason %#08x", uint32(reason))
	}
}

// TestWARPEnv checks that TANGO_D3D_WARP=1 restricts createDevice to WARP.
func TestWARPEnv(t *testing.T) {
	t.Setenv(warpEnv, "1")
	dev, ctx, _, err := createDevice(false)
	if err != nil {
		t.Fatalf("createDevice with %s=1: %v", warpEnv, err)
	}
	defer release(dev)
	defer release(ctx)
	if !isWARP(dev) {
		d, _ := adapterOf(dev)
		t.Errorf("device runs on %q, want WARP", d.adapterName())
	}
}

// TestCreateDeviceDefault creates a device the normal way: hardware if
// present, otherwise WARP.
func TestCreateDeviceDefault(t *testing.T) {
	dev, ctx, level, err := createDevice(false)
	if err != nil {
		t.Fatalf("createDevice: %v", err)
	}
	defer release(dev)
	defer release(ctx)
	d, _ := adapterOf(dev)
	t.Logf("adapter %q, feature level %#x, WARP %v", d.adapterName(), level, isWARP(dev))
}
