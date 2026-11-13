package windows

import "encoding/binary"

// TaskDialogIndirect takes a TASKDIALOGCONFIG, which commctrl.h declares
// inside #pragma pack(1): its fields follow each other with no padding, so
// a Go struct cannot describe it. It is encoded field by field instead,
// for 64-bit Windows (8-byte pointers and handles).

// taskDialogConfig holds the TASKDIALOGCONFIG members the backend sets;
// every other member is encoded as zero. Pointers and handles are plain
// addresses here: the caller keeps what they point at alive.
type taskDialogConfig struct {
	Parent          uintptr // hwndParent
	Instance        uintptr // hInstance
	Flags           uint32  // dwFlags (TDF_*)
	WindowTitle     uintptr // pszWindowTitle
	MainIcon        uintptr // pszMainIcon (a TD_*_ICON resource ID)
	MainInstruction uintptr // pszMainInstruction
	Content         uintptr // pszContent, 0 for none
	ButtonCount     uint32  // cButtons
	Buttons         uintptr // pButtons, see taskDialogButtons
	DefaultButton   int32   // nDefaultButton
}

// taskDialogConfigSize is sizeof(TASKDIALOGCONFIG) on 64-bit Windows.
const taskDialogConfigSize = 160

// packedLE appends little-endian fields with no alignment padding.
type packedLE []byte

func (p *packedLE) u32(v uint32)   { *p = binary.LittleEndian.AppendUint32(*p, v) }
func (p *packedLE) ptr(v uintptr)  { *p = binary.LittleEndian.AppendUint64(*p, uint64(v)) }
func (p *packedLE) zeroPtrs(n int) { *p = append(*p, make([]byte, 8*n)...) }

// encode returns the packed TASKDIALOGCONFIG, cbSize included.
func (c *taskDialogConfig) encode() []byte {
	b := make(packedLE, 0, taskDialogConfigSize)
	b.u32(taskDialogConfigSize) // cbSize
	b.ptr(c.Parent)
	b.ptr(c.Instance)
	b.u32(c.Flags)
	b.u32(0) // dwCommonButtons: only custom buttons are used
	b.ptr(c.WindowTitle)
	b.ptr(c.MainIcon)
	b.ptr(c.MainInstruction)
	b.ptr(c.Content)
	b.u32(c.ButtonCount)
	b.ptr(c.Buttons)
	b.u32(uint32(c.DefaultButton))
	b.u32(0) // cRadioButtons
	b.ptr(0) // pRadioButtons
	b.u32(0) // nDefaultRadioButton
	// pszVerificationText, pszExpandedInformation, pszExpandedControlText,
	// pszCollapsedControlText, the footer icon, pszFooter, pfCallback and
	// lpCallbackData.
	b.zeroPtrs(8)
	b.u32(0) // cxWidth: let the dialog size itself
	return b
}

// taskDialogButtons encodes a TASKDIALOG_BUTTON array (packed: a 4-byte
// ID then the 8-byte label pointer), giving the buttons consecutive IDs
// from firstID.
func taskDialogButtons(firstID int32, labels []uintptr) []byte {
	b := make(packedLE, 0, 12*len(labels))
	for i, label := range labels {
		b.u32(uint32(firstID) + uint32(i))
		b.ptr(label)
	}
	return b
}
