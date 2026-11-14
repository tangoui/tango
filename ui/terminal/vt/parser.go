package vt

import "unicode/utf8"

// Handler receives the actions decoded by a [Parser]. Slices passed to a
// method are only valid during the call.
type Handler interface {
	// Print receives one printable code point that is not ASCII, or
	// U+FFFD for each byte of invalid UTF-8.
	Print(r rune)
	// PrintASCII receives a run of printable ASCII (0x20–0x7E). A run may
	// be split across several calls.
	PrintASCII(b []byte)
	// Execute receives a C0 control byte. 0x80–0x9F are never C1 controls
	// because input is UTF-8.
	Execute(b byte)
	// CSI receives a control sequence. private is '<', '=', '>', '?' or 0.
	CSI(p *Params, inter []byte, final byte, private byte)
	// ESC receives an escape sequence.
	ESC(inter []byte, final byte)
	// OSC receives an operating system command without its BEL or ST
	// terminator. Strings longer than 1 MB are dropped.
	OSC(data []byte)
	// DCS receives a device control string once its ST arrives. A private
	// marker ('<'–'?') is the first byte of inter. Payloads longer than
	// 1 MB are dropped.
	DCS(p *Params, inter []byte, final byte, data []byte)
}

const (
	maxParams    = 32
	maxSubParams = 8 // including the main value
	maxInter     = 4
	maxParamVal  = 1<<31 - 1
	// maxStringLen bounds OSC and DCS payloads.
	maxStringLen = 1 << 20
)

// Params holds the parameters of a CSI or DCS sequence: up to 32 parameters
// separated by ';', each with up to 8 values separated by ':'. Extra
// parameters and values are ignored.
type Params struct {
	v [maxParams][maxSubParams]int
	n [maxParams]uint8 // values in v[i], 1..maxSubParams
	k int              // number of parameters
	// skip drops digits after a value that exceeded the limits, until the
	// next separator that opens a value within them.
	skip bool
}

// Len returns the number of parameters.
func (p *Params) Len() int { return p.k }

// Get returns the main value of parameter i, or def if it is missing or 0.
func (p *Params) Get(i, def int) int {
	if i < 0 || i >= p.k || p.v[i][0] == 0 {
		return def
	}
	return p.v[i][0]
}

// Sub returns the main value of parameter i followed by its ':'
// sub-parameters, or nil if i is out of range. Missing values are 0.
func (p *Params) Sub(i int) []int {
	if i < 0 || i >= p.k {
		return nil
	}
	return p.v[i][:p.n[i]]
}

func (p *Params) reset() {
	p.k = 0
	p.skip = false
}

// add feeds one parameter byte: a digit, ';' or ':'.
func (p *Params) add(c byte) {
	if p.k == 0 {
		p.k, p.n[0], p.v[0][0] = 1, 1, 0
	}
	i := p.k - 1
	switch c {
	case ';':
		if p.k == maxParams {
			p.skip = true
			return
		}
		p.k++
		p.n[i+1], p.v[i+1][0] = 1, 0
		p.skip = false
	case ':':
		if p.skip || p.n[i] == maxSubParams {
			p.skip = true
			return
		}
		p.v[i][p.n[i]] = 0
		p.n[i]++
	default:
		if p.skip {
			return
		}
		v := &p.v[i][p.n[i]-1]
		// Clamp before multiplying so *v*10 cannot overflow a 32-bit int.
		if *v > (maxParamVal-9)/10 {
			*v = maxParamVal
		} else {
			*v = min(*v*10+int(c-'0'), maxParamVal)
		}
	}
}

// Parser decodes a byte stream into [Handler] calls. It is not safe for
// concurrent use.
type Parser struct {
	h     Handler
	state state

	params  Params
	inter   [maxInter]byte
	nInter  int
	private byte
	final   byte // DCS final byte
	// tooLong marks a sequence whose intermediates or payload exceeded the
	// limits; it is consumed but not dispatched.
	tooLong bool
	buf     []byte // OSC or DCS payload

	// afterString is set when ESC ended an OSC, DCS or ignored string, so
	// that the '\' completing ST is swallowed instead of dispatched.
	afterString bool

	utf  [utf8.UTFMax]byte // incomplete UTF-8 sequence from the last Write
	nUTF int
}

// NewParser returns a Parser in the ground state that reports to h.
func NewParser(h Handler) *Parser {
	return &Parser{h: h}
}

// Write parses b. Input may be split across calls at any byte.
func (p *Parser) Write(b []byte) {
	i := 0
	if p.nUTF > 0 {
		i = p.finishUTF8(b)
	}
	for i < len(b) {
		if p.state == sGround {
			j := i
			for j < len(b) && b[j]-0x20 < 0x5F { // 0x20–0x7E
				j++
			}
			if j > i {
				p.h.PrintASCII(b[i:j])
				i = j
				continue
			}
			if b[i] >= 0x80 {
				if !utf8.FullRune(b[i:]) {
					p.nUTF = copy(p.utf[:], b[i:])
					return
				}
				r, size := utf8.DecodeRune(b[i:])
				p.h.Print(r)
				i += size
				continue
			}
		}
		p.step(b[i])
		i++
	}
}

// finishUTF8 completes the UTF-8 sequence left over from the previous Write
// and returns how many bytes of b it consumed. It decodes exactly as
// utf8.DecodeRune would over the unsplit input.
func (p *Parser) finishUTF8(b []byte) int {
	i := 0
	for p.nUTF > 0 {
		var tmp [utf8.UTFMax]byte
		n := copy(tmp[:], p.utf[:p.nUTF])
		m := copy(tmp[n:], b[i:])
		if !utf8.FullRune(tmp[:n+m]) {
			// Only possible when b[i:] is shorter than the rest of the rune.
			copy(p.utf[n:], b[i:])
			p.nUTF += m
			return len(b)
		}
		r, size := utf8.DecodeRune(tmp[:n+m])
		p.h.Print(r)
		if size >= n {
			i += size - n
			p.nUTF = 0
		} else {
			// Invalid: the remaining saved bytes start the next decode.
			p.nUTF = copy(p.utf[:], p.utf[size:p.nUTF])
		}
	}
	return i
}

func (p *Parser) step(c byte) {
	if p.afterString {
		p.afterString = false
		if p.state == sEscape && c == '\\' {
			p.state = sGround
			return
		}
	}
	e := table[p.state][c]
	switch e.action() {
	case aNone:
	case aExecute:
		p.h.Execute(c)
	case aCollect:
		if p.nInter < maxInter {
			p.inter[p.nInter] = c
			p.nInter++
		} else {
			p.tooLong = true
		}
	case aPrivate:
		p.private = c
	case aParam:
		p.params.add(c)
	case aESCDispatch:
		if !p.tooLong {
			p.h.ESC(p.inter[:p.nInter], c)
		}
	case aCSIDispatch:
		if !p.tooLong {
			p.h.CSI(&p.params, p.inter[:p.nInter], c, p.private)
		}
	case aHook:
		p.final = c
	case aPut:
		if p.tooLong {
			break
		}
		if len(p.buf) >= maxStringLen {
			p.tooLong = true
			p.buf = p.buf[:0]
			break
		}
		p.buf = append(p.buf, c)
	case aOSCEnd:
		if !p.tooLong {
			p.h.OSC(p.buf)
		}
	case aDCSEnd:
		if !p.tooLong {
			p.h.DCS(&p.params, p.inter[:p.nInter], p.final, p.buf)
		}
	}
	next := e.next()
	if next == noChange {
		return
	}
	p.state = next
	switch next {
	case sEscape, sCSIEntry, sDCSEntry, sOSC:
		p.clear()
	}
	if next == sEscape {
		switch e.action() {
		case aOSCEnd, aDCSEnd, aStringEnd:
			p.afterString = true
		}
	}
}

// clear resets the per-sequence state on entry to a new sequence.
func (p *Parser) clear() {
	p.params.reset()
	p.nInter = 0
	p.private = 0
	p.final = 0
	p.tooLong = false
	if cap(p.buf) > 64<<10 {
		p.buf = nil // do not keep a large payload buffer alive
	} else {
		p.buf = p.buf[:0]
	}
}
