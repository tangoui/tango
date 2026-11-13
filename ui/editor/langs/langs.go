// Package langs embeds the built-in language data of the code editor: Monaco
// Monarch grammars, language configurations and file associations, plus the
// four Monaco base themes.
//
// The JSON under data/ is generated from monaco-editor@0.56.0 by gen/export.mjs
// (see gen/README.md and NOTICE); nothing here depends on JavaScript at run
// time. Regular expressions are stored as {"$regex": source, "flags": flags}
// and decode to RegexSpec.
package langs

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed data/*.json
var dataFS embed.FS

// Language is one registered language id with its grammar and configuration.
type Language struct {
	ID               string
	Extensions       []string
	Filenames        []string
	FilenamePatterns []string
	Aliases          []string
	MimeTypes        []string
	// FirstLine is a JavaScript regular expression source matched against the
	// first line of a file ("" when the language declares none).
	FirstLine string
	// Grammar is the Monarch language definition, verbatim from the export.
	Grammar json.RawMessage
	// Conf is the decoded subset of the language configuration.
	Conf Conf
	// ConfRaw is the full language configuration, verbatim from the export
	// (Monaco's LanguageConfiguration JSON, regular expressions as
	// {"$regex", "flags"}). It includes fields Conf does not model.
	ConfRaw json.RawMessage

	order    int              // position in Monaco's registration sequence
	patterns []*regexp.Regexp // compiled FilenamePatterns, lazily
	patOnce  sync.Once
}

// Conf is the subset of Monaco's LanguageConfiguration the editor uses.
type Conf struct {
	LineComment      string
	BlockComment     [2]string
	Brackets         [][2]string
	AutoClosingPairs []Pair
	SurroundingPairs []Pair
	WordPattern      *RegexSpec
	Indentation      struct{ Increase, Decrease, IndentNextLine, Unindented *RegexSpec }
	Folding          struct {
		OffSide    bool
		Start, End *RegexSpec
	}
	// OnEnterRules are the language's onEnterRules, in order (the first
	// rule whose patterns all match wins).
	OnEnterRules []OnEnterRule
	// AutoCloseBefore is the language's autoCloseBefore: the characters
	// before which a pair is auto-closed when the auto-closing options are
	// "languageDefined". Empty when the language sets none (Monaco then
	// uses its defaults); no built-in language sets it to "".
	AutoCloseBefore string
	// ColorizedBracketPairs is the language's colorizedBracketPairs, nil
	// when it sets none (Monaco then colorizes its brackets).
	ColorizedBracketPairs [][2]string
}

// OnEnterRule is one of Monaco's onEnterRules: when the text before the
// caret matches BeforeText and, when set, the text after it matches
// AfterText and the line above matches PreviousLineText, Enter applies
// Action.
type OnEnterRule struct {
	BeforeText, AfterText, PreviousLineText *RegexSpec
	Action                                  EnterAction
}

// EnterAction is what an onEnter rule does (Monaco's EnterAction). Indent
// is the IndentAction as a string: "none" (keep the indentation),
// "indent" (one level deeper), "indentOutdent" (an indented line for the
// caret and the rest on a line at the old indentation) or "outdent" (one
// level shallower). AppendText is appended after the new line's
// indentation; RemoveText removes that many characters from it.
type EnterAction struct {
	Indent     string
	AppendText string
	RemoveText int
}

// Monaco's IndentAction values in the exported JSON (an enum: 0..3).
var indentActions = [...]string{"none", "indent", "indentOutdent", "outdent"}

func (r *OnEnterRule) UnmarshalJSON(b []byte) error {
	var v struct {
		BeforeText       *RegexSpec `json:"beforeText"`
		AfterText        *RegexSpec `json:"afterText"`
		PreviousLineText *RegexSpec `json:"previousLineText"`
		Action           struct {
			IndentAction *int   `json:"indentAction"`
			AppendText   string `json:"appendText"`
			RemoveText   int    `json:"removeText"`
		} `json:"action"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("onEnterRule: %w", err)
	}
	if v.BeforeText == nil {
		return errors.New(`onEnterRule: missing "beforeText"`)
	}
	a := v.Action
	if a.IndentAction == nil || *a.IndentAction < 0 || *a.IndentAction >= len(indentActions) {
		return errors.New("onEnterRule: bad indentAction")
	}
	*r = OnEnterRule{
		BeforeText:       v.BeforeText,
		AfterText:        v.AfterText,
		PreviousLineText: v.PreviousLineText,
		Action:           EnterAction{Indent: indentActions[*a.IndentAction], AppendText: a.AppendText, RemoveText: a.RemoveText},
	}
	return nil
}

// Pair is an auto-closing or surrounding pair. NotIn lists token scopes
// ("string", "comment") in which an auto-closing pair is not inserted.
type Pair struct {
	Open, Close string
	NotIn       []string
}

// RegexSpec is a JavaScript regular expression as exported from Monaco:
// JSON {"$regex": source, "flags": flags}.
type RegexSpec struct {
	Source, Flags string
}

func (r RegexSpec) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Source string `json:"$regex"`
		Flags  string `json:"flags"`
	}{r.Source, r.Flags})
}

func (r *RegexSpec) UnmarshalJSON(b []byte) error {
	var v struct {
		Source *string `json:"$regex"`
		Flags  string  `json:"flags"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("regex: %w", err)
	}
	if v.Source == nil {
		return errors.New(`regex: missing "$regex"`)
	}
	*r = RegexSpec{Source: *v.Source, Flags: v.Flags}
	return nil
}

func (p *Pair) UnmarshalJSON(b []byte) error {
	var v struct {
		Open  string   `json:"open"`
		Close string   `json:"close"`
		NotIn []string `json:"notIn"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*p = Pair{Open: v.Open, Close: v.Close, NotIn: v.NotIn}
	return nil
}

// jsonConf mirrors Monaco's LanguageConfiguration JSON shape.
type jsonConf struct {
	Comments *struct {
		LineComment  string    `json:"lineComment"`
		BlockComment [2]string `json:"blockComment"`
	} `json:"comments"`
	Brackets         [][2]string `json:"brackets"`
	AutoClosingPairs []Pair      `json:"autoClosingPairs"`
	SurroundingPairs []Pair      `json:"surroundingPairs"`
	WordPattern      *RegexSpec  `json:"wordPattern"`
	IndentationRules *struct {
		Increase       *RegexSpec `json:"increaseIndentPattern"`
		Decrease       *RegexSpec `json:"decreaseIndentPattern"`
		IndentNextLine *RegexSpec `json:"indentNextLinePattern"`
		Unindented     *RegexSpec `json:"unIndentedLinePattern"`
	} `json:"indentationRules"`
	Folding *struct {
		OffSide bool `json:"offSide"`
		Markers *struct {
			Start *RegexSpec `json:"start"`
			End   *RegexSpec `json:"end"`
		} `json:"markers"`
	} `json:"folding"`
	OnEnterRules          []json.RawMessage `json:"onEnterRules"`
	AutoCloseBefore       *string           `json:"autoCloseBefore"`
	ColorizedBracketPairs [][2]string       `json:"colorizedBracketPairs"`
}

func (c *Conf) UnmarshalJSON(b []byte) error {
	var j jsonConf
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*c = Conf{
		Brackets:         j.Brackets,
		AutoClosingPairs: j.AutoClosingPairs,
		SurroundingPairs: j.SurroundingPairs,
		WordPattern:      j.WordPattern,

		ColorizedBracketPairs: j.ColorizedBracketPairs,
	}
	// A malformed onEnter rule is skipped, not the whole configuration
	// (Monaco ignores what it cannot use).
	for _, raw := range j.OnEnterRules {
		var r OnEnterRule
		if err := json.Unmarshal(raw, &r); err == nil {
			c.OnEnterRules = append(c.OnEnterRules, r)
		}
	}
	if j.AutoCloseBefore != nil {
		c.AutoCloseBefore = *j.AutoCloseBefore
	}
	if j.Comments != nil {
		c.LineComment = j.Comments.LineComment
		c.BlockComment = j.Comments.BlockComment
	}
	if r := j.IndentationRules; r != nil {
		c.Indentation.Increase = r.Increase
		c.Indentation.Decrease = r.Decrease
		c.Indentation.IndentNextLine = r.IndentNextLine
		c.Indentation.Unindented = r.Unindented
	}
	if f := j.Folding; f != nil {
		c.Folding.OffSide = f.OffSide
		if f.Markers != nil {
			c.Folding.Start = f.Markers.Start
			c.Folding.End = f.Markers.End
		}
	}
	return nil
}

// languageFile is the shape of data/<id>.json.
type languageFile struct {
	RegistrationOrder int `json:"registrationOrder"`
	Registration      struct {
		ID               string   `json:"id"`
		Extensions       []string `json:"extensions"`
		Filenames        []string `json:"filenames"`
		FilenamePatterns []string `json:"filenamePatterns"`
		Aliases          []string `json:"aliases"`
		MimeTypes        []string `json:"mimetypes"`
		FirstLine        string   `json:"firstLine"`
	} `json:"registration"`
	Language json.RawMessage `json:"language"`
	Conf     json.RawMessage `json:"conf"`
}

const themesFile = "themes.json"

var (
	loadOnce  sync.Once
	languages []*Language          // sorted by ID
	byID      map[string]*Language // id -> language
	byOrder   []*Language          // Monaco registration order
	themes    map[string]json.RawMessage
)

// load parses the embedded data once. The data is generated and covered by
// tests, so a decoding failure is a build defect and panics.
func load() {
	loadOnce.Do(func() {
		entries, err := dataFS.ReadDir("data")
		if err != nil {
			panic(err)
		}
		byID = make(map[string]*Language, len(entries))
		for _, e := range entries {
			name := e.Name()
			raw, err := dataFS.ReadFile("data/" + name)
			if err != nil {
				panic(err)
			}
			if name == themesFile {
				if err := json.Unmarshal(raw, &themes); err != nil {
					panic(fmt.Sprintf("langs: %s: %v", name, err))
				}
				continue
			}
			var f languageFile
			if err := json.Unmarshal(raw, &f); err != nil {
				panic(fmt.Sprintf("langs: %s: %v", name, err))
			}
			var conf Conf
			if err := json.Unmarshal(f.Conf, &conf); err != nil {
				panic(fmt.Sprintf("langs: %s: conf: %v", name, err))
			}
			r := f.Registration
			if r.ID+".json" != name {
				panic(fmt.Sprintf("langs: %s declares id %q", name, r.ID))
			}
			l := &Language{
				ID:               r.ID,
				Extensions:       r.Extensions,
				Filenames:        r.Filenames,
				FilenamePatterns: r.FilenamePatterns,
				Aliases:          r.Aliases,
				MimeTypes:        r.MimeTypes,
				FirstLine:        r.FirstLine,
				Grammar:          f.Language,
				Conf:             conf,
				ConfRaw:          f.Conf,
				order:            f.RegistrationOrder,
			}
			languages = append(languages, l)
			byID[l.ID] = l
		}
		sort.Slice(languages, func(i, j int) bool { return languages[i].ID < languages[j].ID })
		byOrder = append([]*Language(nil), languages...)
		sort.SliceStable(byOrder, func(i, j int) bool { return byOrder[i].order < byOrder[j].order })
	})
}

// Builtin returns all built-in languages sorted by ID. The slice and its
// elements are shared; callers must not modify them.
func Builtin() []*Language {
	load()
	return languages
}

// ByID returns the built-in language with the given id, or nil.
func ByID(id string) *Language {
	load()
	return byID[id]
}

// BaseThemes returns Monaco's base themes "vs", "vs-dark", "hc-black" and
// "hc-light", each as JSON {base, inherit, rules, colors}. The map is shared;
// callers must not modify it.
func BaseThemes() map[string]json.RawMessage {
	load()
	return themes
}

// ForFilename returns the language associated with a file name or path, or
// nil. It follows Monaco's association rules: the path is compared
// case-insensitively; an exact file name match beats a filename pattern,
// which beats an extension; among patterns and extensions the longest match
// wins; remaining ties go to the language registered last.
func ForFilename(name string) *Language {
	load()
	p := strings.ToLower(strings.ReplaceAll(name, `\`, "/"))
	base := path.Base(p)
	if p == "" || base == "/" || base == "." {
		return nil
	}
	var (
		patternMatch, extMatch *Language
		patternLen, extLen     int
	)
	for i := len(byOrder) - 1; i >= 0; i-- {
		l := byOrder[i]
		for _, f := range l.Filenames {
			if strings.EqualFold(f, base) {
				return l
			}
		}
		for j, pat := range l.FilenamePatterns {
			if patternMatch != nil && len(pat) <= patternLen {
				continue
			}
			target := base
			if strings.Contains(pat, "/") {
				target = p
			}
			if re := l.compiledPatterns()[j]; re != nil && re.MatchString(target) {
				patternMatch, patternLen = l, len(pat)
			}
		}
		for _, ext := range l.Extensions {
			if (extMatch == nil || len(ext) > extLen) && strings.HasSuffix(base, strings.ToLower(ext)) {
				extMatch, extLen = l, len(ext)
			}
		}
	}
	if patternMatch != nil {
		return patternMatch
	}
	return extMatch
}

func (l *Language) compiledPatterns() []*regexp.Regexp {
	l.patOnce.Do(func() {
		l.patterns = make([]*regexp.Regexp, len(l.FilenamePatterns))
		for i, pat := range l.FilenamePatterns {
			l.patterns[i] = globRegexp(pat)
		}
	})
	return l.patterns
}

// globRegexp compiles the glob syntax Monaco/VS Code accepts in
// filenamePatterns (*, ?, **, {a,b}, [...]) into a case-insensitive regexp.
// It returns nil for a pattern it cannot translate.
func globRegexp(glob string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString(`(?i)^`)
	inGroup := 0
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				i++
				if i+1 < len(glob) && glob[i+1] == '/' {
					i++
					b.WriteString(`(?:.*/)?`) // "**/" matches zero or more directories
				} else {
					b.WriteString(`.*`)
				}
			} else {
				b.WriteString(`[^/]*`)
			}
		case '?':
			b.WriteString(`[^/]`)
		case '{':
			inGroup++
			b.WriteString(`(?:`)
		case '}':
			if inGroup == 0 {
				b.WriteString(`\}`)
				continue
			}
			inGroup--
			b.WriteString(`)`)
		case ',':
			if inGroup > 0 {
				b.WriteString(`|`)
			} else {
				b.WriteString(`,`)
			}
		case '[':
			end := strings.IndexByte(glob[i+1:], ']')
			if end < 0 {
				b.WriteString(`\[`)
				continue
			}
			class := glob[i+1 : i+1+end]
			if strings.HasPrefix(class, "!") {
				class = "^" + class[1:]
			}
			b.WriteString("[" + class + "]")
			i += end + 1
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	if inGroup != 0 {
		return nil
	}
	b.WriteString(`$`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil
	}
	return re
}
