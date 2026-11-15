package ui

import (
	"bytes"
	"strings"
	"sync"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// LogBuffer: the lines a LogView shows.

// LogLevel is the severity of a log line. LogView filters by a minimum
// level; lines whose level is unknown (LogNone) always pass.
type LogLevel uint8

// The levels, lowest first.
const (
	LogNone LogLevel = iota
	LogTrace
	LogDebug
	LogInfo
	LogWarn
	LogError
)

// logLine is one stored line: plain text, its ANSI style spans, its width
// in cells and its level.
type logLine struct {
	text  string
	spans []ANSISpan
	cells int32
	level LogLevel // effective: a line without a level of its own continues the previous one
	own   bool     // the level was found in this line
}

// LogBuffer holds log lines for LogView: escape sequences are parsed and
// levels detected once, when a line is added. It is safe for concurrent
// use: a goroutine reading a process's output can Write to it while the UI
// thread shows it (call the OnChange function to wake the UI).
//
// With a line limit the oldest lines are dropped as new ones arrive.
type LogBuffer struct {
	mu       sync.Mutex
	lines    []logLine // live lines are lines[head:]
	head     int
	dropped  int64 // lines dropped from the front so far
	max      int
	ver      uint64 // bumps on Clear
	parser   ANSIParser
	partial  []byte
	level    LogLevel
	detect   func(line string) LogLevel
	onChange func()
}

// NewLogBuffer returns a buffer keeping at most maxLines lines (<= 0: no
// limit).
func NewLogBuffer(maxLines int) *LogBuffer { return &LogBuffer{max: max(0, maxLines)} }

// SetDetector replaces the level detector (default DetectLogLevel). It
// applies to lines added afterwards.
func (b *LogBuffer) SetDetector(fn func(line string) LogLevel) {
	b.mu.Lock()
	b.detect = fn
	b.mu.Unlock()
}

// OnChange sets a function called (outside the buffer's lock, from the
// goroutine that added lines) after lines are added or the buffer is
// cleared; pass a Context.Invalidator to repaint the view.
func (b *LogBuffer) OnChange(fn func()) {
	b.mu.Lock()
	b.onChange = fn
	b.mu.Unlock()
}

// Write appends p as output: complete lines are added, an unterminated
// tail waits for the next Write (or Flush). It implements io.Writer.
func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	n := len(p)
	added := false
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			b.partial = append(b.partial, p...)
			break
		}
		line := p[:i]
		if len(b.partial) > 0 {
			line = append(b.partial, line...)
			b.partial = b.partial[:0]
		}
		b.addLocked(LogNone, string(bytes.TrimSuffix(line, []byte{'\r'})))
		added = true
		p = p[i+1:]
	}
	fn := b.onChange
	b.mu.Unlock()
	if added && fn != nil {
		fn()
	}
	return n, nil
}

// Flush adds the unterminated tail of the written output as a line.
func (b *LogBuffer) Flush() {
	b.mu.Lock()
	added := len(b.partial) > 0
	if added {
		b.addLocked(LogNone, string(b.partial))
		b.partial = b.partial[:0]
	}
	fn := b.onChange
	b.mu.Unlock()
	if added && fn != nil {
		fn()
	}
}

// Append adds lines (each may hold "\n", which splits it further).
func (b *LogBuffer) Append(lines ...string) { b.AppendLevel(LogNone, lines...) }

// AppendLevel adds lines with a known level (LogNone: detect it).
func (b *LogBuffer) AppendLevel(level LogLevel, lines ...string) {
	b.mu.Lock()
	for _, s := range lines {
		for part := range strings.SplitSeq(s, "\n") {
			b.addLocked(level, strings.TrimSuffix(part, "\r"))
		}
	}
	fn := b.onChange
	b.mu.Unlock()
	if fn != nil && len(lines) > 0 {
		fn()
	}
}

func (b *LogBuffer) addLocked(level LogLevel, s string) {
	var ln logLine
	ln.text, ln.spans = b.parser.Line(s, nil)
	ln.cells = int32(txvCells(ln.text))
	if level == LogNone {
		if b.detect != nil {
			level = b.detect(ln.text)
		} else {
			level = DetectLogLevel(ln.text)
		}
	}
	if level != LogNone {
		ln.level, ln.own = level, true
		b.level = level
	} else {
		ln.level = b.level
	}
	b.lines = append(b.lines, ln)
	if b.max > 0 && len(b.lines)-b.head > b.max {
		k := len(b.lines) - b.head - b.max
		b.head += k
		b.dropped += int64(k)
		if b.head > len(b.lines)/2 && b.head > 1024 {
			// Compact into a new array: readers keep the old one.
			b.lines = append(make([]logLine, 0, len(b.lines)-b.head+b.max/4+16), b.lines[b.head:]...)
			b.head = 0
		}
	}
}

// Clear removes every line.
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	b.dropped += int64(len(b.lines) - b.head)
	b.lines, b.head = nil, 0
	b.partial = b.partial[:0]
	b.parser.Reset()
	b.level = LogNone
	b.ver++
	fn := b.onChange
	b.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Len returns the number of lines.
func (b *LogBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.lines) - b.head
}

// Line returns the plain text of line i (0 is the oldest kept line).
func (b *LogBuffer) Line(i int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if i < 0 || i >= len(b.lines)-b.head {
		return ""
	}
	return b.lines[b.head+i].text
}

// Level returns the level of line i (inherited from the line above when
// the line has none of its own).
func (b *LogBuffer) Level(i int) LogLevel {
	b.mu.Lock()
	defer b.mu.Unlock()
	if i < 0 || i >= len(b.lines)-b.head {
		return LogNone
	}
	return b.lines[b.head+i].level
}

// logSnap is an immutable view of the buffer: lines[0] has absolute index
// base (lines dropped before it count too).
type logSnap struct {
	lines []logLine
	base  int64
	ver   uint64
}

func (b *LogBuffer) snapshot() logSnap {
	b.mu.Lock()
	defer b.mu.Unlock()
	live := b.lines[b.head:len(b.lines):len(b.lines)]
	return logSnap{lines: live, base: b.dropped, ver: b.ver}
}

// DetectLogLevel finds a level word near the start of a line, within the
// first 120 bytes and not part of a longer word: TRACE, DEBUG, INFO, WARN,
// WARNING, ERROR, FATAL, PANIC, CRITICAL, SEVERE in any case, and the
// upper-case abbreviations TRC, DBG, INF, WRN, ERR, CRIT.
func DetectLogLevel(line string) LogLevel {
	s := line
	if len(s) > 120 {
		s = s[:120]
	}
	for i := 0; i < len(s); {
		c := s[i]
		if !logIsLetter(c) {
			i++
			continue
		}
		j := i + 1
		for j < len(s) && logIsLetter(s[j]) {
			j++
		}
		if j-i <= 8 && (i == 0 || !logIsWordByte(s[i-1])) && (j == len(s) || !logIsWordByte(s[j])) {
			if lv := logLevelWord(s[i:j]); lv != LogNone {
				return lv
			}
		}
		i = j
	}
	return LogNone
}

func logIsLetter(c byte) bool { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }
func logIsWordByte(c byte) bool {
	return logIsLetter(c) || (c >= '0' && c <= '9') || c == '_' || c >= utf8.RuneSelf
}

func logLevelWord(w string) LogLevel {
	switch w {
	case "TRC":
		return LogTrace
	case "DBG":
		return LogDebug
	case "INF":
		return LogInfo
	case "WRN":
		return LogWarn
	case "ERR", "CRIT":
		return LogError
	}
	var buf [8]byte
	for i := 0; i < len(w); i++ {
		buf[i] = w[i] | 0x20
	}
	switch string(buf[:len(w)]) {
	case "trace":
		return LogTrace
	case "debug":
		return LogDebug
	case "info":
		return LogInfo
	case "warn", "warning":
		return LogWarn
	case "error", "fatal", "panic", "critical", "severe":
		return LogError
	}
	return LogNone
}
