package automation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/a11y"
)

// Query selects nodes of a semantics tree. All set fields must match.
type Query struct {
	// Role is an a11y.Role name ("button", "textbox"); "" matches any.
	Role string `json:"role,omitempty"`
	// Name is compared with Node.Name according to Match; "" matches any
	// name.
	Name string `json:"name,omitempty"`
	// Match is how Name compares: "" or "exact" (equal), "contains"
	// (case-insensitive substring), "regexp" (Go regular expression,
	// unanchored).
	Match string `json:"match,omitempty"`
	// Nth picks among the matches in reading order: 0 the first, 1 the
	// second; negative counts from the end (-1 the last).
	Nth int `json:"nth,omitempty"`
	// States lists a11y.State names that must be set ("checked"), or not
	// set with a leading "!" ("!disabled").
	States []string `json:"states,omitempty"`
	// Value, when set, must equal Node.Value.
	Value *string `json:"value,omitempty"`
}

// Match kinds.
const (
	MatchExact    = "exact"
	MatchContains = "contains"
	MatchRegexp   = "regexp"
)

// String describes q for error messages: `button "OK"`, `textbox ~"mail"`.
func (q Query) String() string {
	var b strings.Builder
	if q.Role == "" {
		b.WriteString("node")
	} else {
		b.WriteString(q.Role)
	}
	if q.Name != "" {
		switch q.Match {
		case MatchContains:
			b.WriteString(" containing ")
		case MatchRegexp:
			b.WriteString(" matching /")
			b.WriteString(q.Name)
			b.WriteString("/")
		default:
			b.WriteString(" ")
		}
		if q.Match != MatchRegexp {
			b.WriteString(strconv.Quote(q.Name))
		}
	}
	for _, s := range q.States {
		b.WriteString(" [" + s + "]")
	}
	if q.Value != nil {
		b.WriteString(" value=" + strconv.Quote(*q.Value))
	}
	if q.Nth != 0 {
		fmt.Fprintf(&b, " #%d", q.Nth)
	}
	return b.String()
}

// Matcher is a compiled Query.
type Matcher struct {
	anyRole bool
	role    a11y.Role
	name    func(string) bool
	with    a11y.State
	without a11y.State
	value   *string
	nth     int
}

// Compile checks q and prepares it for matching.
func (q Query) Compile() (*Matcher, error) {
	m := &Matcher{anyRole: q.Role == "", value: q.Value, nth: q.Nth}
	if !m.anyRole {
		r, ok := a11y.ParseRole(q.Role)
		if !ok {
			return nil, fmt.Errorf("automation: unknown role %q", q.Role)
		}
		m.role = r
	}
	switch q.Match {
	case "", MatchExact:
		if q.Name != "" {
			want := q.Name
			m.name = func(s string) bool { return s == want }
		}
	case MatchContains:
		if q.Name != "" {
			want := strings.ToLower(q.Name)
			m.name = func(s string) bool { return strings.Contains(strings.ToLower(s), want) }
		}
	case MatchRegexp:
		re, err := regexp.Compile(q.Name)
		if err != nil {
			return nil, fmt.Errorf("automation: bad name pattern: %v", err)
		}
		m.name = re.MatchString
	default:
		return nil, fmt.Errorf("automation: unknown match %q (exact, contains, regexp)", q.Match)
	}
	for _, s := range q.States {
		neg := strings.HasPrefix(s, "!")
		st, ok := ParseState(strings.TrimPrefix(s, "!"))
		if !ok {
			return nil, fmt.Errorf("automation: unknown state %q", s)
		}
		if neg {
			m.without |= st
		} else {
			m.with |= st
		}
	}
	return m, nil
}

// Matches reports whether n passes every filter (Nth aside).
func (m *Matcher) Matches(n *a11y.Node) bool {
	if n == nil {
		return false
	}
	if !m.anyRole && n.Role != m.role {
		return false
	}
	if m.name != nil && !m.name(n.Name) {
		return false
	}
	if n.States&m.with != m.with || n.States&m.without != 0 {
		return false
	}
	if m.value != nil && n.Value != *m.value {
		return false
	}
	return true
}

// All returns the matching nodes of t in reading order (depth first from
// the root).
func (m *Matcher) All(t *a11y.Tree) []*a11y.Node {
	var out []*a11y.Node
	t.Walk(func(n *a11y.Node, _ int) bool {
		if m.Matches(n) {
			out = append(out, n)
		}
		return true
	})
	return out
}

// Pick returns the node Nth selects (nil when there are too few matches)
// and the number of matches.
func (m *Matcher) Pick(t *a11y.Tree) (*a11y.Node, int) {
	all := m.All(t)
	i := m.nth
	if i < 0 {
		i += len(all)
	}
	if i < 0 || i >= len(all) {
		return nil, len(all)
	}
	return all[i], len(all)
}

// ParseState returns the single a11y.State named s (as State.String
// prints it).
func ParseState(s string) (a11y.State, bool) {
	for i := 0; i < 32; i++ {
		st := a11y.State(1) << i
		if st.String() == s && s != "" {
			return st, true
		}
	}
	return 0, false
}

// ParseAction returns the single a11y.Action named s ("press",
// "setValue").
func ParseAction(s string) (a11y.Action, bool) {
	for i := 0; i < 16; i++ {
		a := a11y.Action(1) << i
		if a.String() == s && s != "" {
			return a, true
		}
	}
	return 0, false
}
