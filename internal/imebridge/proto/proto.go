// Package proto defines version 2 of the wire protocol shared by IME bridges.
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	Version    = 2
	MaxFrame   = 64 * 1024
	headerSize = 16
)

type Type uint16

const (
	TypeHello Type = iota + 1
	TypeSessionOpen
	TypeSessionClose
	TypeKey
	TypeTestKey
	TypeFocus
	TypeMenuOpen
	TypeMenuPick
	TypeModePick
	TypeKeyResult
	TypeCompose
	TypeCommit
	TypeMode
	TypeMenu
)

const (
	KeyDown uint8 = 1 << iota
	KeyUp
	KeyRepeat
	KeyExtended
)

const (
	ModShift uint8 = 1 << iota
	ModCtrl
	ModAlt
	ModMeta
	ModCapsLock
)

type Message struct {
	Seq  uint64
	Body any // one of the message types below
}

type Hello struct {
	PID          uint32
	Exe          string
	AppContainer bool
	Integrity    uint32
}

type SessionOpen uint64
type SessionClose uint64
type MenuOpen uint64
type MenuPick string
type ModePick string

type Rect struct {
	X, Y, Width, Height int32 // physical screen pixels on Windows
}

type Key struct {
	Session     uint64
	VK, Scan    uint16
	Flags, Mods uint8
	Text        string
	Caret       Rect
}

type TestKey Key

type Focus struct {
	Session uint64
	On      bool
}

type KeyResult struct {
	Session uint64
	Eaten   bool
}

type Compose struct {
	Session uint64
	Text    string
	Cursor  uint32 // rune index
}

type Commit struct {
	Session uint64
	Text    string
}

type Mode struct {
	ID, Label, Short, Icon string
}

type MenuItem struct {
	ID, Title                    string
	Checked, Disabled, Separator bool
}

type Menu struct {
	Items []MenuItem
}

var errFrame = errors.New("imebridge: invalid frame")

// Marshal encodes exactly one length-prefixed frame.
func Marshal(msg Message) ([]byte, error) {
	e := encoder{data: make([]byte, headerSize)}
	var kind Type
	switch v := msg.Body.(type) {
	case Hello:
		kind = TypeHello
		e.u32(v.PID)
		e.flag(v.AppContainer)
		e.u32(v.Integrity)
		e.text(v.Exe)
	case SessionOpen:
		kind = TypeSessionOpen
		e.u64(uint64(v))
	case SessionClose:
		kind = TypeSessionClose
		e.u64(uint64(v))
	case Key:
		kind = TypeKey
		e.key(v)
	case TestKey:
		kind = TypeTestKey
		e.key(Key(v))
	case Focus:
		kind = TypeFocus
		e.u64(v.Session)
		e.flag(v.On)
	case MenuOpen:
		kind = TypeMenuOpen
		e.u64(uint64(v))
	case MenuPick:
		kind = TypeMenuPick
		e.text(string(v))
	case ModePick:
		kind = TypeModePick
		e.text(string(v))
	case KeyResult:
		kind = TypeKeyResult
		e.u64(v.Session)
		e.flag(v.Eaten)
	case Compose:
		kind = TypeCompose
		e.u64(v.Session)
		e.u32(v.Cursor)
		e.text(v.Text)
	case Commit:
		kind = TypeCommit
		e.u64(v.Session)
		e.text(v.Text)
	case Mode:
		kind = TypeMode
		e.text(v.ID)
		e.text(v.Label)
		e.text(v.Short)
		e.text(v.Icon)
	case Menu:
		kind = TypeMenu
		if len(v.Items) > (MaxFrame-headerSize-2)/5 {
			return nil, errFrame
		}
		e.u16(uint16(len(v.Items)))
		for _, item := range v.Items {
			var flags byte
			if item.Checked {
				flags |= 1
			}
			if item.Disabled {
				flags |= 2
			}
			if item.Separator {
				flags |= 4
			}
			e.byte(flags)
			e.text(item.ID)
			e.text(item.Title)
		}
	default:
		return nil, fmt.Errorf("imebridge: unknown message body %T", v)
	}
	if e.invalid || len(e.data) > MaxFrame {
		return nil, errFrame
	}
	binary.LittleEndian.PutUint32(e.data, uint32(len(e.data)-4))
	binary.LittleEndian.PutUint16(e.data[4:], uint16(kind))
	binary.LittleEndian.PutUint16(e.data[6:], Version)
	binary.LittleEndian.PutUint64(e.data[8:], msg.Seq)
	return e.data, nil
}

// Read decodes one frame from a stream and rejects unknown types and versions.
func Read(r io.Reader) (Message, error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:4]); err != nil {
		return Message{}, err
	}
	n := binary.LittleEndian.Uint32(header[:4])
	if n < headerSize-4 || n > MaxFrame-4 {
		return Message{}, errFrame
	}
	if _, err := io.ReadFull(r, header[4:]); err != nil {
		return Message{}, err
	}
	if binary.LittleEndian.Uint16(header[6:]) != Version {
		return Message{}, errFrame
	}
	payload := make([]byte, int(n)-(headerSize-4))
	if _, err := io.ReadFull(r, payload); err != nil {
		return Message{}, err
	}
	d := decoder{data: payload}
	var body any
	switch Type(binary.LittleEndian.Uint16(header[4:])) {
	case TypeHello:
		body = Hello{PID: d.u32(), AppContainer: d.flag(), Integrity: d.u32(), Exe: d.text()}
	case TypeSessionOpen:
		body = SessionOpen(d.u64())
	case TypeSessionClose:
		body = SessionClose(d.u64())
	case TypeKey:
		body = d.key()
	case TypeTestKey:
		body = TestKey(d.key())
	case TypeFocus:
		body = Focus{Session: d.u64(), On: d.flag()}
	case TypeMenuOpen:
		body = MenuOpen(d.u64())
	case TypeMenuPick:
		body = MenuPick(d.text())
	case TypeModePick:
		body = ModePick(d.text())
	case TypeKeyResult:
		body = KeyResult{Session: d.u64(), Eaten: d.flag()}
	case TypeCompose:
		body = Compose{Session: d.u64(), Cursor: d.u32(), Text: d.text()}
	case TypeCommit:
		body = Commit{Session: d.u64(), Text: d.text()}
	case TypeMode:
		body = Mode{ID: d.text(), Label: d.text(), Short: d.text(), Icon: d.text()}
	case TypeMenu:
		n := int(d.u16())
		if n > len(d.data)/5 {
			return Message{}, errFrame
		}
		items := make([]MenuItem, n)
		for i := range items {
			flags := d.byte()
			d.invalid = d.invalid || flags&^7 != 0
			items[i] = MenuItem{
				ID: d.text(), Title: d.text(), Checked: flags&1 != 0,
				Disabled: flags&2 != 0, Separator: flags&4 != 0,
			}
		}
		body = Menu{Items: items}
	default:
		return Message{}, errFrame
	}
	if d.invalid || len(d.data) != 0 {
		return Message{}, errFrame
	}
	return Message{Seq: binary.LittleEndian.Uint64(header[8:]), Body: body}, nil
}

type encoder struct {
	data    []byte
	invalid bool
}

func (e *encoder) byte(v byte)  { e.data = append(e.data, v) }
func (e *encoder) u16(v uint16) { e.data = binary.LittleEndian.AppendUint16(e.data, v) }
func (e *encoder) u32(v uint32) { e.data = binary.LittleEndian.AppendUint32(e.data, v) }
func (e *encoder) u64(v uint64) { e.data = binary.LittleEndian.AppendUint64(e.data, v) }

func (e *encoder) flag(v bool) {
	if v {
		e.byte(1)
	} else {
		e.byte(0)
	}
}

func (e *encoder) text(s string) {
	if e.invalid || len(s) > MaxFrame || !utf8.ValidString(s) {
		e.invalid = true
		return
	}
	text := utf16.Encode([]rune(s))
	if 2+2*len(text) > MaxFrame-len(e.data) {
		e.invalid = true
		return
	}
	e.u16(uint16(len(text)))
	for _, u := range text {
		e.u16(u)
	}
}

func (e *encoder) key(k Key) {
	e.u64(k.Session)
	e.u16(k.VK)
	e.u16(k.Scan)
	e.byte(k.Flags)
	e.byte(k.Mods)
	e.u32(uint32(k.Caret.X))
	e.u32(uint32(k.Caret.Y))
	e.u32(uint32(k.Caret.Width))
	e.u32(uint32(k.Caret.Height))
	e.text(k.Text)
}

type decoder struct {
	data    []byte
	invalid bool
}

func (d *decoder) take(n int) []byte {
	if len(d.data) < n {
		d.invalid = true
		return nil
	}
	part := d.data[:n]
	d.data = d.data[n:]
	return part
}

func (d *decoder) byte() byte {
	if b := d.take(1); b != nil {
		return b[0]
	}
	return 0
}

func (d *decoder) u16() uint16 {
	if b := d.take(2); b != nil {
		return binary.LittleEndian.Uint16(b)
	}
	return 0
}

func (d *decoder) u32() uint32 {
	if b := d.take(4); b != nil {
		return binary.LittleEndian.Uint32(b)
	}
	return 0
}

func (d *decoder) u64() uint64 {
	if b := d.take(8); b != nil {
		return binary.LittleEndian.Uint64(b)
	}
	return 0
}

func (d *decoder) flag() bool {
	v := d.byte()
	d.invalid = d.invalid || v > 1
	return v == 1
}

func (d *decoder) text() string {
	n := int(d.u16())
	bytes := d.take(2 * n)
	if bytes == nil || n == 0 {
		return ""
	}
	text := make([]uint16, n)
	for i := range text {
		text[i] = binary.LittleEndian.Uint16(bytes[2*i:])
	}
	for i := 0; i < n; i++ {
		u := text[i]
		if u >= 0xd800 && u <= 0xdbff {
			i++
			if i < n && text[i] >= 0xdc00 && text[i] <= 0xdfff {
				continue
			}
			d.invalid = true
			break
		}
		if u >= 0xdc00 && u <= 0xdfff {
			d.invalid = true
			break
		}
	}
	return string(utf16.Decode(text))
}

func (d *decoder) key() Key {
	return Key{
		Session: d.u64(), VK: d.u16(), Scan: d.u16(), Flags: d.byte(), Mods: d.byte(),
		Caret: Rect{int32(d.u32()), int32(d.u32()), int32(d.u32()), int32(d.u32())},
		Text:  d.text(),
	}
}
