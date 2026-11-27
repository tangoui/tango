package windows

const (
	overlayExStyle       = 0x08000000 | 0x00000008 | 0x00000080 // NOACTIVATE, TOPMOST, TOOLWINDOW
	overlayStyle         = 0x80000000 | 0x02000000              // POPUP, CLIPCHILDREN
	overlayShowCmd       = 4                                    // SW_SHOWNOACTIVATE
	overlayMouseActivate = 3                                    // MA_NOACTIVATE
)
