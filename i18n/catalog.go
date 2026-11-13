package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// message is one catalog entry: a plain string, or plural forms keyed by
// CLDR category ("one", "other", ...).
type message struct {
	text   string
	plural map[string]string
}

type catalog struct {
	mu    sync.RWMutex
	msgs  map[string]map[string]message // locale -> key -> message
	langs atomic.Pointer[[]string]      // sorted locales of msgs
}

var cat = &catalog{msgs: map[string]map[string]message{}}

func (c *catalog) langsSnapshot() []string {
	if p := c.langs.Load(); p != nil {
		return *p
	}
	return nil
}

// setLangs publishes the locale list; c.mu is held.
func (c *catalog) setLangs(msgs map[string]map[string]message) {
	l := make([]string, 0, len(msgs))
	for k := range msgs {
		l = append(l, k)
	}
	slices.Sort(l)
	c.langs.Store(&l)
}

// Load reads every "<locale>.json" file in dir of fsys into the catalog,
// e.g. "locales/en.json", "locales/zh-CN.json" (the file name is the BCP 47
// tag; "zh_CN" is accepted too). Each file is one flat JSON object mapping a
// key to either a string or, for plurals, an object of CLDR categories:
//
//	{
//	  "hello": "Hello, {name}!",
//	  "files": {"one": "{count} file", "other": "{count} files"}
//	}
//
// Keys of a locale loaded again are replaced one by one, so an app can load
// a base set and then overrides. Load can be called at any time; listeners
// are notified if the effective locale changes because Supported did.
func Load(fsys fs.FS, dir string) error {
	if dir == "" {
		dir = "."
	}
	ents, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("i18n: %w", err)
	}
	loaded := map[string]map[string]message{}
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || !strings.EqualFold(path.Ext(name), ".json") {
			continue
		}
		loc := Canonical(strings.TrimSuffix(name, path.Ext(name)))
		if loc == "" {
			continue
		}
		data, err := fs.ReadFile(fsys, path.Join(dir, name))
		if err != nil {
			return fmt.Errorf("i18n: %w", err)
		}
		msgs, err := parseCatalog(data)
		if err != nil {
			return fmt.Errorf("i18n: %s: %w", path.Join(dir, name), err)
		}
		loaded[loc] = msgs
	}
	cat.mu.Lock()
	for loc, msgs := range loaded {
		m := cat.msgs[loc]
		if m == nil {
			m = map[string]message{}
			cat.msgs[loc] = m
		}
		for k, v := range msgs {
			m[k] = v
		}
	}
	cat.setLangs(cat.msgs)
	cat.mu.Unlock()
	update(func() {})
	return nil
}

func parseCatalog(data []byte) (map[string]message, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]message, len(raw))
	for k, v := range raw {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			out[k] = message{text: s}
			continue
		}
		var forms map[string]string
		if err := json.Unmarshal(v, &forms); err != nil {
			return nil, fmt.Errorf("key %q: want a string or an object of plural forms", k)
		}
		for c := range forms {
			if !validCategory(c) {
				return nil, fmt.Errorf("key %q: unknown plural category %q", k, c)
			}
		}
		if _, ok := forms["other"]; !ok {
			return nil, fmt.Errorf("key %q: plural forms need \"other\"", k)
		}
		out[k] = message{plural: forms}
	}
	return out, nil
}

func validCategory(c string) bool {
	switch c {
	case "zero", "one", "two", "few", "many", "other":
		return true
	}
	return false
}

// lookup finds key for locale, falling back to DefaultLocale. It returns
// the locale the message came from.
func lookup(locale, key string) (message, string, bool) {
	cat.mu.RLock()
	defer cat.mu.RUnlock()
	if m, ok := cat.msgs[locale][key]; ok {
		return m, locale, true
	}
	if m, ok := cat.msgs[DefaultLocale][key]; ok {
		return m, DefaultLocale, true
	}
	return message{}, "", false
}

// T returns the message for key in the current locale with named arguments
// filled in. Arguments are name/value pairs:
//
//	i18n.T("hello", "name", "Tom") // "Hello, {name}!" -> "Hello, Tom!"
//
// A missing key falls back to DefaultLocale and then to the key itself; in
// development builds (-tags tango_dev, as tan dev builds) each missing key is
// reported once on the log. For a plural entry T uses the "other" form.
func T(key string, args ...any) string {
	loc := Locale()
	m, _, ok := lookup(loc, key)
	if !ok {
		warnMissing(loc, key)
		return format(key, args)
	}
	if m.plural != nil {
		return format(m.plural["other"], args)
	}
	return format(m.text, args)
}

// N returns the plural form of key for count in the current locale, with
// "count" and the name/value pairs in args available as arguments:
//
//	i18n.N("files", 3) // {"one": "{count} file", "other": "{count} files"} -> "3 files"
//
// The category comes from the plural rule of the locale the message was
// found in (see PluralCategory); a missing category uses "other". A plain
// string entry is used for every count.
func N(key string, count int, args ...any) string {
	loc := Locale()
	args = append([]any{"count", count}, args...)
	m, from, ok := lookup(loc, key)
	if !ok {
		warnMissing(loc, key)
		return format(key, args)
	}
	if m.plural == nil {
		return format(m.text, args)
	}
	s, ok := m.plural[PluralCategory(from, count)]
	if !ok {
		s = m.plural["other"]
	}
	return format(s, args)
}

// format replaces {name} with the matching argument. "{{" and "}}" stand
// for literal braces; a placeholder without an argument is left as is.
func format(s string, args []any) string {
	if !strings.ContainsAny(s, "{}") {
		return s
	}
	if len(args)%2 != 0 {
		badArgs(args)
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c == '{' || c == '}') && i+1 < len(s) && s[i+1] == c {
			b.WriteByte(c)
			i++
			continue
		}
		if c == '{' {
			if j := strings.IndexByte(s[i+1:], '}'); j >= 0 {
				name := s[i+1 : i+1+j]
				if v, ok := arg(args, name); ok {
					b.WriteString(v)
					i += j + 1
					continue
				}
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

func arg(args []any, name string) (string, bool) {
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok && k == name {
			switch v := args[i+1].(type) {
			case string:
				return v, true
			case int:
				return strconv.Itoa(v), true
			default:
				return fmt.Sprint(v), true
			}
		}
	}
	return "", false
}

var warned sync.Map // "locale\x00key" -> true

func warnMissing(locale, key string) {
	if !debug {
		return
	}
	if _, dup := warned.LoadOrStore(locale+"\x00"+key, true); !dup {
		log.Printf("tango/i18n: missing message %q for locale %s", key, locale)
	}
}

func badArgs(args []any) {
	if !debug {
		return
	}
	k := fmt.Sprintf("args\x00%d\x00%v", len(args), args[0])
	if _, dup := warned.LoadOrStore(k, true); !dup {
		log.Printf("tango/i18n: odd number of arguments %v; want name/value pairs", args)
	}
}
