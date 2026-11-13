//go:build !darwin && !windows && !linux

package text

// No discoverable fonts (js/wasm, plan9, BSDs without a known layout): apps
// must call RegisterFont. Until then text is measured approximately and not
// drawn.
func systemFamilies() (sans, mono []*family) { return nil, nil }
