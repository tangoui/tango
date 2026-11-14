package vt

// The transition table follows Paul Williams' DEC ANSI parser
// (vt100.net/emu/dec_ansi_parser) with these changes:
//
//   - Input is UTF-8, so bytes 0x80–0xFF never act as C1 controls. In
//     the ground state the parser decodes them before consulting the
//     table; in OSC and DCS payloads they are collected as data; in every
//     other state they are ignored.
//   - ':' is a sub-parameter separator in CSI and DCS parameters instead of
//     sending the sequence to the ignore state.
//   - BEL ends an OSC string, as in xterm.
//   - CAN and SUB abort an OSC or DCS string without dispatching it.
//   - ESC ends OSC, DCS and SOS/PM/APC strings; a following '\' completes
//     the ST and is swallowed (see Parser.afterString).

type state uint8

const (
	sGround state = iota
	sEscape
	sEscInter
	sCSIEntry
	sCSIParam
	sCSIInter
	sCSIIgnore
	sOSC
	sDCSEntry
	sDCSParam
	sDCSInter
	sDCSPass
	sDCSIgnore
	sString // SOS, PM and APC: consumed and discarded
	numStates

	noChange state = 0xF // table entry: stay in the current state
)

type action uint8

const (
	aNone        action = iota
	aExecute            // Handler.Execute
	aCollect            // append an intermediate byte
	aPrivate            // record a CSI private marker
	aParam              // digit, ';' or ':'
	aESCDispatch        // Handler.ESC
	aCSIDispatch        // Handler.CSI
	aHook               // record the DCS final byte; payload follows
	aPut                // append an OSC or DCS payload byte
	aOSCEnd             // Handler.OSC
	aDCSEnd             // Handler.DCS
	aStringEnd          // end of an ignored string
)

// entry packs an action (high nibble) and the next state (low nibble).
type entry uint8

func (e entry) action() action { return action(e >> 4) }
func (e entry) next() state    { return state(e & 0xF) }

// table is indexed by [state][byte].
var table = buildTable()

func buildTable() *[numStates][256]entry {
	t := new([numStates][256]entry)
	set := func(s state, lo, hi byte, a action, next state) {
		for c := int(lo); c <= int(hi); c++ {
			t[s][c] = entry(a)<<4 | entry(next)
		}
	}
	// executeC0 executes C0 controls in place; CAN, SUB and ESC are
	// overridden by the "anywhere" transitions at the end.
	executeC0 := func(s state) {
		set(s, 0x00, 0x1F, aExecute, noChange)
	}
	ignore := func(s state, lo, hi byte) { set(s, lo, hi, aNone, noChange) }

	for s := range numStates {
		ignore(s, 0x00, 0xFF)
	}

	executeC0(sGround)

	executeC0(sEscape)
	set(sEscape, 0x20, 0x2F, aCollect, sEscInter)
	set(sEscape, 0x30, 0x7E, aESCDispatch, sGround)
	set(sEscape, 'P', 'P', aNone, sDCSEntry)
	set(sEscape, '[', '[', aNone, sCSIEntry)
	set(sEscape, ']', ']', aNone, sOSC)
	set(sEscape, 'X', 'X', aNone, sString)
	set(sEscape, '^', '^', aNone, sString)
	set(sEscape, '_', '_', aNone, sString)

	executeC0(sEscInter)
	set(sEscInter, 0x20, 0x2F, aCollect, noChange)
	set(sEscInter, 0x30, 0x7E, aESCDispatch, sGround)

	executeC0(sCSIEntry)
	set(sCSIEntry, 0x20, 0x2F, aCollect, sCSIInter)
	set(sCSIEntry, 0x30, 0x3B, aParam, sCSIParam)
	set(sCSIEntry, 0x3C, 0x3F, aPrivate, sCSIParam)
	set(sCSIEntry, 0x40, 0x7E, aCSIDispatch, sGround)

	executeC0(sCSIParam)
	set(sCSIParam, 0x20, 0x2F, aCollect, sCSIInter)
	set(sCSIParam, 0x30, 0x3B, aParam, noChange)
	set(sCSIParam, 0x3C, 0x3F, aNone, sCSIIgnore)
	set(sCSIParam, 0x40, 0x7E, aCSIDispatch, sGround)

	executeC0(sCSIInter)
	set(sCSIInter, 0x20, 0x2F, aCollect, noChange)
	set(sCSIInter, 0x30, 0x3F, aNone, sCSIIgnore)
	set(sCSIInter, 0x40, 0x7E, aCSIDispatch, sGround)

	executeC0(sCSIIgnore)
	set(sCSIIgnore, 0x40, 0x7E, aNone, sGround)

	set(sOSC, 0x20, 0xFF, aPut, noChange)
	set(sOSC, 0x07, 0x07, aOSCEnd, sGround)
	set(sOSC, 0x1B, 0x1B, aOSCEnd, sEscape)

	set(sDCSEntry, 0x20, 0x2F, aCollect, sDCSInter)
	set(sDCSEntry, 0x30, 0x3B, aParam, sDCSParam)
	set(sDCSEntry, 0x3C, 0x3F, aCollect, sDCSParam)
	set(sDCSEntry, 0x40, 0x7E, aHook, sDCSPass)

	set(sDCSParam, 0x20, 0x2F, aCollect, sDCSInter)
	set(sDCSParam, 0x30, 0x3B, aParam, noChange)
	set(sDCSParam, 0x3C, 0x3F, aNone, sDCSIgnore)
	set(sDCSParam, 0x40, 0x7E, aHook, sDCSPass)

	set(sDCSInter, 0x20, 0x2F, aCollect, noChange)
	set(sDCSInter, 0x30, 0x3F, aNone, sDCSIgnore)
	set(sDCSInter, 0x40, 0x7E, aHook, sDCSPass)

	set(sDCSPass, 0x00, 0x7E, aPut, noChange)
	set(sDCSPass, 0x80, 0xFF, aPut, noChange)
	set(sDCSPass, 0x1B, 0x1B, aDCSEnd, sEscape)

	set(sDCSIgnore, 0x1B, 0x1B, aStringEnd, sEscape)
	set(sString, 0x1B, 0x1B, aStringEnd, sEscape)

	// "Anywhere" transitions. String states set their own ESC above.
	for s := range numStates {
		set(s, 0x18, 0x18, aExecute, sGround)
		set(s, 0x1A, 0x1A, aExecute, sGround)
		if s != sOSC && s != sDCSPass && s != sDCSIgnore && s != sString {
			set(s, 0x1B, 0x1B, aNone, sEscape)
		}
	}
	return t
}
