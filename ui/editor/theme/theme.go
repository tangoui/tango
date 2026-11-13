// Package theme parses Monaco-format editor themes (IStandaloneThemeData:
// {base, inherit, rules, colors}) and resolves Monarch token types such as
// "keyword.go" or "string.escape.ts" to display styles.
//
// Token resolution follows Monaco's TokenTheme: rules form a trie keyed by
// dot-separated segments and a token takes the style of the deepest matching
// node. A node starts as a copy of its parent's style when it is created and
// each rule only overwrites the fields it sets, so a rule that omits
// foreground, background or fontStyle inherits them from its nearest
// ancestor rule, while fontStyle "" explicitly resets to plain.
//
// Parsing is lenient about individual values, unlike Monaco in places: an
// invalid color in the colors table (Monaco shows it as red) or in a rule
// (Monaco throws and the theme fails to apply) is ignored and recorded in
// Theme.Warnings; non-string rule fields are ignored field by field (as
// Monaco's typeof checks do) and non-object rules are skipped. Only malformed
// JSON or a missing/unknown base fails Parse.
//
// The structure follows Monaco's TokenTheme (vs/editor/common/languages/
// supports/tokenization.ts) and StandaloneTheme (vs/editor/standalone/browser/
// standaloneThemeService.ts), Copyright (c) Microsoft Corporation, MIT License.
// See NOTICE in this directory.
package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui/editor/langs"
)

// Style is the resolved display style of a token type.
type Style struct {
	Foreground, Background                 color.NRGBA
	Bold, Italic, Underline, Strikethrough bool
}

// Theme is a parsed, immutable editor theme. It is safe for concurrent use.
type Theme struct {
	base     string
	rules    []rawRule // the theme's own rules, as given (used when inherited from)
	colors   map[string]color.NRGBA
	root     *node
	cache    sync.Map // token type -> Style
	warnings []string
	// plain: the colors are exactly the base theme's, so the registry
	// defaults resolve to the generated final colors (colors.go).
	plain    bool
	defaults sync.Map // color id -> colorResult, for themes that are not plain
}

// colorResult is a resolved registry default.
type colorResult struct {
	c  color.NRGBA
	ok bool
}

// rawRule is one entry of a theme's "rules" array. Pointer fields distinguish
// "not set" from set (fontStyle "" is a reset, not an omission).
type rawRule struct {
	Token      string  `json:"token"`
	Foreground *string `json:"foreground"`
	Background *string `json:"background"`
	FontStyle  *string `json:"fontStyle"`
}

// themeData is the top level of a theme. Rules and colors are decoded by hand
// so that one bad entry or field does not fail the whole theme.
type themeData struct {
	Base    string          `json:"base"`
	Inherit bool            `json:"inherit"`
	Rules   json.RawMessage `json:"rules"`
	Colors  json.RawMessage `json:"colors"`
}

func isNull(raw json.RawMessage) bool {
	s := strings.TrimSpace(string(raw))
	return s == "" || s == "null"
}

// decodeRules decodes the "rules" array leniently: non-object entries are
// skipped and non-string fields are treated as unset, with a warning each.
func decodeRules(raw json.RawMessage, warn func(string, ...any)) []rawRule {
	if isNull(raw) {
		return nil
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		warn("rules: not an array, ignored")
		return nil
	}
	rules := make([]rawRule, 0, len(entries))
	for i, e := range entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(e, &fields); err != nil || fields == nil {
			warn("rules[%d]: not an object, ignored", i)
			continue
		}
		str := func(name string) *string {
			v, ok := fields[name]
			if !ok || isNull(v) {
				return nil
			}
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				warn("rules[%d] %s: not a string (%s), ignored", i, name, v)
				return nil
			}
			return &s
		}
		r := rawRule{Foreground: str("foreground"), Background: str("background"), FontStyle: str("fontStyle")}
		if tok := str("token"); tok != nil {
			r.Token = *tok
		}
		rules = append(rules, r)
	}
	return rules
}

// decodeColors decodes the "colors" object leniently: values that are not
// valid color strings are dropped with a warning.
func decodeColors(raw json.RawMessage, warn func(string, ...any)) map[string]color.NRGBA {
	out := map[string]color.NRGBA{}
	if isNull(raw) {
		return out
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		warn("colors: not an object, ignored")
		return out
	}
	for k, v := range m {
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			warn("color %q: not a string (%s), ignored", k, v)
			continue
		}
		c, err := parseColor(s, true)
		if err != nil {
			warn("color %q: %v, ignored", k, err)
			continue
		}
		out[k] = c
	}
	return out
}

// Font style bits; fontNotSet marks a rule that does not touch font style.
const (
	fontItalic = 1 << iota
	fontBold
	fontUnderline
	fontStrikethrough

	fontNotSet = -1
)

var baseNames = [...]string{"vs", "vs-dark", "hc-black", "hc-light"}

func isBaseName(name string) bool {
	for _, n := range baseNames {
		if n == name {
			return true
		}
	}
	return false
}

var (
	baseOnce   sync.Once
	baseThemes map[string]*Theme
)

// Base returns Monaco's built-in base theme "vs", "vs-dark", "hc-black" or
// "hc-light", or nil for any other name. The result is shared.
func Base(name string) *Theme {
	baseOnce.Do(func() {
		baseThemes = make(map[string]*Theme, len(baseNames))
		for _, n := range baseNames {
			raw, ok := langs.BaseThemes()[n]
			if !ok {
				panic("theme: missing built-in theme " + n)
			}
			t, err := parse(raw, nil)
			if err != nil {
				panic(fmt.Sprintf("theme: built-in %s: %v", n, err))
			}
			baseThemes[n] = t
		}
	})
	return baseThemes[name]
}

// Parse parses a Monaco theme. "base" must be one of the four base themes;
// when "inherit" is true the base theme's rules are applied first and its
// colors fill in keys the theme does not define. Colors are hex RGB, RGBA,
// RRGGBB or RRGGBBAA with an optional leading '#'; token rule colors are
// opaque, so any alpha digits in them are ignored, as in Monaco. Invalid
// individual values are ignored and reported by Warnings (see the package doc).
func Parse(data []byte) (*Theme, error) {
	return parse(data, Base)
}

// parse parses data; baseOf resolves the inherited base theme and is nil
// while the base themes themselves are being built (they never inherit).
func parse(data []byte, baseOf func(string) *Theme) (*Theme, error) {
	var d themeData
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("theme: %w", err)
	}
	if !isBaseName(d.Base) {
		return nil, fmt.Errorf("theme: illegal base %q", d.Base)
	}
	t := &Theme{base: d.Base}
	warn := func(format string, a ...any) { t.warnings = append(t.warnings, fmt.Sprintf(format, a...)) }
	t.colors = decodeColors(d.Colors, warn)
	// Like Monaco, the theme's own editor.foreground/background become the
	// default token colors (a rule after the inherited rules and before the
	// theme's own). Monaco tests the raw values for truthiness; we also require
	// them to be valid colors (invalid ones were dropped with a warning).
	// Read before base colors are merged in: only the theme's own values count.
	ownColor := func(key string) *string {
		c, ok := t.colors[key]
		if !ok {
			return nil
		}
		s := fmt.Sprintf("%02x%02x%02x", c.R, c.G, c.B) // rule colors are opaque
		return &s
	}
	defFG, defBG := ownColor("editor.foreground"), ownColor("editor.background")
	ownRules := decodeRules(d.Rules, warn)
	for i := range ownRules { // drop invalid rule colors (field by field)
		r := &ownRules[i]
		for _, f := range []struct {
			name string
			p    **string
		}{{"foreground", &r.Foreground}, {"background", &r.Background}} {
			if *f.p == nil {
				continue
			}
			if _, err := parseColor(**f.p, false); err != nil {
				warn("rule %q %s: %v, ignored", r.Token, f.name, err)
				*f.p = nil
			}
		}
	}
	sort.Strings(t.warnings) // map iteration order: keep warnings deterministic
	t.rules = ownRules

	var rules []rawRule
	if d.Inherit && baseOf != nil {
		b := baseOf(d.Base)
		if b == nil {
			return nil, fmt.Errorf("theme: base %q unavailable", d.Base)
		}
		rules = append(rules, b.rules...)
		for k, c := range b.colors {
			if _, ok := t.colors[k]; !ok {
				t.colors[k] = c
			}
		}
	}
	if defFG != nil || defBG != nil {
		rules = append(rules, rawRule{Foreground: defFG, Background: defBG})
	}
	rules = append(rules, ownRules...)

	t.root = buildTrie(rules)
	if baseOf == nil {
		t.plain = true // a base theme
	} else if b := baseOf(d.Base); b != nil && len(b.colors) == len(t.colors) {
		t.plain = true
		for k, c := range b.colors {
			if t.colors[k] != c {
				t.plain = false
				break
			}
		}
	}
	return t, nil
}

// Warnings returns the problems Parse ignored: invalid color values, non-string
// rule fields and non-object rules. It returns a new slice on each call.
func (t *Theme) Warnings() []string { return append([]string(nil), t.warnings...) }

// BaseName returns the theme's base: "vs", "vs-dark", "hc-black" or
// "hc-light" (the theme type that selects default colors).
func (t *Theme) BaseName() string { return t.base }

// Style returns the style for a Monarch token type. Results are cached per
// token type; Style is safe to call from multiple goroutines.
func (t *Theme) Style(tokenType string) Style {
	if s, ok := t.cache.Load(tokenType); ok {
		return s.(Style)
	}
	s := t.root.match(tokenType).style()
	t.cache.Store(tokenType, s)
	return s
}

// Color returns the theme color for a workbench color key such as
// "editor.lineHighlightBackground": the theme's own value (with inherit,
// its base theme's values for keys it does not set), else Monaco 0.56's
// registered default for the theme's base type, resolved against the
// theme's colors (colors.go). It reports false for a key without a color:
// one Monaco has no default for there (editor.lineHighlightBackground), or
// that is not a registered color id.
func (t *Theme) Color(key string) (color.NRGBA, bool) {
	if c, ok := t.colors[key]; ok {
		return c, true
	}
	if t.plain {
		return defaultColor(key, baseType(t.base))
	}
	if r, ok := t.defaults.Load(key); ok {
		r := r.(colorResult)
		return r.c, r.ok
	}
	c, ok := resolveDefault(t, key)
	t.defaults.Store(key, colorResult{c, ok})
	return c, ok
}

// OwnColor returns the theme's own value of key (with inherit, including
// its base theme's colors), without the registry defaults: Monaco's
// theme.defines(key) is whether it reports true.
func (t *Theme) OwnColor(key string) (color.NRGBA, bool) {
	c, ok := t.colors[key]
	return c, ok
}

// parsedRule is a rule with its colors decoded. fg/bg are nil when unset.
type parsedRule struct {
	token     string
	index     int
	fontStyle int
	fg, bg    *color.NRGBA
}

// nodeStyle is the style stored at a trie node.
type nodeStyle struct {
	fontStyle int
	fg, bg    color.NRGBA
}

func (s nodeStyle) style() Style {
	return Style{
		Foreground:    s.fg,
		Background:    s.bg,
		Italic:        s.fontStyle&fontItalic != 0,
		Bold:          s.fontStyle&fontBold != 0,
		Underline:     s.fontStyle&fontUnderline != 0,
		Strikethrough: s.fontStyle&fontStrikethrough != 0,
	}
}

func (s *nodeStyle) overwrite(r parsedRule) {
	if r.fontStyle != fontNotSet {
		s.fontStyle = r.fontStyle
	}
	if r.fg != nil {
		s.fg = *r.fg
	}
	if r.bg != nil {
		s.bg = *r.bg
	}
}

type node struct {
	nodeStyle
	children map[string]*node
}

// split cuts a token at its first '.'.
func split(token string) (head, tail string) {
	head, tail, _ = strings.Cut(token, ".")
	return head, tail
}

func (n *node) match(token string) nodeStyle {
	for token != "" {
		head, tail := split(token)
		child, ok := n.children[head]
		if !ok {
			break
		}
		n, token = child, tail
	}
	return n.nodeStyle
}

func (n *node) insert(r parsedRule) {
	token := r.token
	for token != "" {
		head, tail := split(token)
		child, ok := n.children[head]
		if !ok {
			// A new node starts from its parent's current style.
			child = &node{nodeStyle: n.nodeStyle}
			if n.children == nil {
				n.children = make(map[string]*node)
			}
			n.children[head] = child
		}
		n, token = child, tail
	}
	n.overwrite(r)
}

// buildTrie resolves rules the way Monaco does: sort by token then source
// order (so ancestors are inserted before descendants and later rules win),
// fold leading "" rules into the defaults, then insert the rest.
// Rule colors must already be valid (parse drops invalid ones).
func buildTrie(raw []rawRule) *node {
	rules := make([]parsedRule, len(raw))
	for i, r := range raw {
		p := parsedRule{token: r.Token, index: i, fontStyle: fontNotSet}
		if r.FontStyle != nil {
			p.fontStyle = parseFontStyle(*r.FontStyle)
		}
		p.fg, p.bg = parseRuleColor(r.Foreground), parseRuleColor(r.Background)
		rules[i] = p
	}
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].token < rules[j].token })

	black := color.NRGBA{A: 0xff}
	white := color.NRGBA{0xff, 0xff, 0xff, 0xff}
	root := &node{nodeStyle: nodeStyle{fontStyle: 0, fg: black, bg: white}}
	for _, r := range rules {
		root.insert(r)
	}
	return root
}

func parseFontStyle(s string) int {
	fs := 0
	for _, seg := range strings.Split(s, " ") {
		switch seg {
		case "italic":
			fs |= fontItalic
		case "bold":
			fs |= fontBold
		case "underline":
			fs |= fontUnderline
		case "strikethrough":
			fs |= fontStrikethrough
		}
	}
	return fs
}

// parseRuleColor decodes an already-validated rule color; nil when unset or invalid.
func parseRuleColor(s *string) *color.NRGBA {
	if s == nil {
		return nil
	}
	c, err := parseColor(*s, false)
	if err != nil {
		return nil
	}
	return &c
}

var errColor = errors.New("want hex RGB, RGBA, RRGGBB or RRGGBBAA")

// parseColor parses RGB, RGBA, RRGGBB or RRGGBBAA hex with an optional '#'. When
// keepAlpha is false the alpha digits are accepted but the result is opaque.
func parseColor(s string, keepAlpha bool) (color.NRGBA, error) {
	h := strings.TrimPrefix(s, "#")
	var v [8]uint8
	for i := 0; i < len(h); i++ {
		d, ok := hexDigit(h[i])
		if !ok || i >= len(v) {
			return color.NRGBA{}, fmt.Errorf("%q: %w", s, errColor)
		}
		v[i] = d
	}
	var c color.NRGBA
	switch len(h) {
	case 3:
		c = color.NRGBA{v[0] * 0x11, v[1] * 0x11, v[2] * 0x11, 0xff}
	case 4:
		c = color.NRGBA{v[0] * 0x11, v[1] * 0x11, v[2] * 0x11, v[3] * 0x11}
		if !keepAlpha {
			c.A = 0xff
		}
	case 6:
		c = color.NRGBA{v[0]<<4 | v[1], v[2]<<4 | v[3], v[4]<<4 | v[5], 0xff}
	case 8:
		c = color.NRGBA{v[0]<<4 | v[1], v[2]<<4 | v[3], v[4]<<4 | v[5], v[6]<<4 | v[7]}
		if !keepAlpha {
			c.A = 0xff
		}
	default:
		return color.NRGBA{}, fmt.Errorf("%q: %w", s, errColor)
	}
	return c, nil
}

func hexDigit(b byte) (uint8, bool) {
	switch {
	case '0' <= b && b <= '9':
		return b - '0', true
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10, true
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}
