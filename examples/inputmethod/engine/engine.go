// Package engine is the input engine: it turns keystrokes into text to
// show while composing (preedit) and text to commit. This first version is
// a toy with a tiny built-in dictionary; it exists to prove the path from
// the system's input method framework to Go and back.
package engine

// Special names the non-character keys the engine cares about.
type Special int

const (
	None Special = iota
	Space
	Enter
	Backspace
	Escape
)

// Key is one keystroke: a character (Rune) or a Special key.
type Key struct {
	Rune    rune
	Special Special
}

// Result is what the host must do after a keystroke. When Handled is false
// the host passes the key on to the application untouched. Otherwise it
// first inserts Commit (if any), then shows Preedit as the marked text
// ("" removes the marked text).
type Result struct {
	Handled    bool
	Commit     string
	Preedit    string
	Candidates []string
}

var toyDict = map[string][]string{
	"nihao":    {"你好", "拟好"},
	"zhongguo": {"中国"},
	"shijie":   {"世界", "视界"},
	"tango":    {"探戈"},
	"shurufa":  {"输入法"},
	"ni":       {"你", "泥", "尼"},
	"hao":      {"好", "号", "豪"},
	"wo":       {"我", "握"},
}

var punct = map[rune]string{
	',': "，", '.': "。", '?': "？", '!': "！", ';': "；", ':': "：",
}

// Composer holds one input session's state. Not safe for concurrent use;
// the host drives it from the main thread.
type Composer struct {
	buf []rune
}

// New returns an empty Composer.
func New() *Composer { return &Composer{} }

// Composing reports whether there is text being composed.
func (c *Composer) Composing() bool { return len(c.buf) > 0 }

// Pick commits the candidate at index and ends composition.
func (c *Composer) Pick(index int) (string, bool) {
	words := toyDict[string(c.buf)]
	if index < 0 || index >= len(words) {
		return "", false
	}
	c.buf = c.buf[:0]
	return words[index], true
}

// Flush ends composition and returns the raw letters to commit (used when
// the input field loses focus).
func (c *Composer) Flush() string {
	s := string(c.buf)
	c.buf = c.buf[:0]
	return s
}

// Feed processes one keystroke.
func (c *Composer) Feed(k Key) Result {
	if k.Special == None && k.Rune >= 'a' && k.Rune <= 'z' {
		c.buf = append(c.buf, k.Rune)
		return c.composing()
	}
	if !c.Composing() {
		return Result{}
	}
	switch k.Special {
	case Space:
		return c.commit(c.best())
	case Enter:
		return c.commit(string(c.buf))
	case Escape:
		c.buf = c.buf[:0]
		return Result{Handled: true}
	case Backspace:
		c.buf = c.buf[:len(c.buf)-1]
		return c.composing()
	}
	tail := string(k.Rune)
	if p, ok := punct[k.Rune]; ok {
		tail = p
	}
	return c.commit(c.best() + tail)
}

func (c *Composer) best() string {
	if cands := toyDict[string(c.buf)]; len(cands) > 0 {
		return cands[0]
	}
	return string(c.buf)
}

func (c *Composer) composing() Result {
	return Result{Handled: true, Preedit: string(c.buf), Candidates: toyDict[string(c.buf)]}
}

func (c *Composer) commit(s string) Result {
	c.buf = c.buf[:0]
	return Result{Handled: true, Commit: s}
}
