package ui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Value sources of JSONView.

// ValueKind is the type of a value tree node.
type ValueKind uint8

// The value kinds. ValueOther is anything else a source wants to show
// with a summary (a function, a channel, a debugger's opaque value).
const (
	ValueNull ValueKind = iota
	ValueBool
	ValueNumber
	ValueString
	ValueObject
	ValueArray
	ValueOther
)

// ValueNode is one node of a value tree.
type ValueNode struct {
	// Name is the object key or variable name; for array elements the
	// index (with Index set). The single root of a JSON document has
	// none.
	Name  string
	Index bool
	Kind  ValueKind
	// Value is the display text: scalars as written (strings without
	// quotes, possibly shortened), a summary for ValueOther; empty for
	// objects and arrays (the view shows their size).
	Value string
	// Type is an optional type label shown after the value (debuggers).
	Type string
	// Count is the number of children of an object or array; -1 when
	// unknown until they are loaded. A node with Count 0 has no children.
	Count int
	// Ref is the source's own handle of the node, passed back to
	// Children.
	Ref any
}

// ValueSource supplies a value tree lazily: JSONView asks for children
// only when a node is expanded, and only for the expanded range of a large
// array or object (groups of 100). Sources may load asynchronously:
// Children returns ok=false while loading and the view asks again (call
// Context.Invalidator's function when the data arrives to repaint at once).
type ValueSource interface {
	// Roots returns the top-level nodes.
	Roots() []ValueNode
	// Children returns the children [from, to) of parent (to = -1: all).
	Children(parent *ValueNode, from, to int) (kids []ValueNode, ok bool)
}

// ValueCopier is implemented by sources whose nodes show a shortened
// value: CopyValue returns the full value for "copy value".
type ValueCopier interface {
	CopyValue(n *ValueNode) string
}

// ---------------------------------------------------------------------------
// JSON source: indexes the document lazily, one container at a time.

// jsonCheckpoint is the stride of the member offsets kept per container,
// so a range deep inside a large array is found without rescanning it.
const jsonCheckpoint = 100

// jsonValueMax is how many runes of a string value a row shows.
const jsonValueMax = 500

type jsonSource struct {
	data []byte
	mu   sync.Mutex
	ckpt map[int][]int // container offset -> offset of members 0, 100, 200 …
}

// JSONSource returns a ValueSource over a JSON document, or an error when
// it is not valid JSON. The document is scanned lazily: expanding a node
// reads only that container, so multi-megabyte documents open at once.
// data must not change while the source is in use.
func JSONSource(data []byte) (ValueSource, error) {
	if !json.Valid(data) {
		var v any
		err := json.Unmarshal(data, &v)
		if err == nil {
			err = fmt.Errorf("invalid JSON")
		}
		return nil, err
	}
	return &jsonSource{data: data, ckpt: map[int][]int{}}, nil
}

func (s *jsonSource) Roots() []ValueNode {
	i := jsonWS(s.data, 0)
	if i >= len(s.data) {
		return nil
	}
	return []ValueNode{s.node(i)}
}

// node describes the value at offset i.
func (s *jsonSource) node(i int) ValueNode {
	d := s.data
	n := ValueNode{Ref: i}
	switch d[i] {
	case '{':
		n.Kind = ValueObject
		_, n.Count = jsonSkip(d, i)
	case '[':
		n.Kind = ValueArray
		_, n.Count = jsonSkip(d, i)
	case '"':
		n.Kind = ValueString
		end, _ := jsonSkip(d, i)
		n.Value = jsonShort(jsonString(d[i:end]), jsonValueMax)
	case 't', 'f':
		n.Kind, n.Value = ValueBool, string(d[i:i+jsonLitLen(d[i])])
	case 'n':
		n.Kind, n.Value = ValueNull, "null"
	default:
		n.Kind = ValueNumber
		end, _ := jsonSkip(d, i)
		n.Value = string(d[i:end])
	}
	return n
}

func jsonLitLen(c byte) int {
	switch c {
	case 't':
		return 4
	case 'f':
		return 5
	}
	return 4
}

func (s *jsonSource) Children(parent *ValueNode, from, to int) ([]ValueNode, bool) {
	off, ok := parent.Ref.(int)
	if !ok || off >= len(s.data) || (s.data[off] != '{' && s.data[off] != '[') {
		return nil, true
	}
	if to < 0 || to > parent.Count {
		to = parent.Count
	}
	if from >= to {
		return nil, true
	}
	d := s.data
	obj := d[off] == '{'
	s.mu.Lock()
	cp, have := s.ckpt[off]
	if !have {
		cp = s.index(off)
		s.ckpt[off] = cp
	}
	s.mu.Unlock()
	k := from / jsonCheckpoint
	if k >= len(cp) {
		return nil, true
	}
	i, idx := cp[k], k*jsonCheckpoint
	out := make([]ValueNode, 0, to-from)
	for idx < to && i < len(d) {
		var name string
		if obj {
			end, _ := jsonSkip(d, i)
			name = jsonString(d[i:end])
			i = jsonWS(d, end)
			i = jsonWS(d, i+1) // ':'
		}
		end, _ := jsonSkip(d, i)
		if idx >= from {
			n := s.node(i)
			if obj {
				n.Name = name
			} else {
				n.Name, n.Index = strconv.Itoa(idx), true
			}
			out = append(out, n)
		}
		idx++
		i = jsonWS(d, end)
		if i < len(d) && d[i] == ',' {
			i = jsonWS(d, i+1)
		}
	}
	return out, true
}

// index returns the offsets of every jsonCheckpoint-th member of the
// container at off (the key for objects, the value for arrays).
func (s *jsonSource) index(off int) []int {
	d := s.data
	obj := d[off] == '{'
	var cp []int
	i := jsonWS(d, off+1)
	for idx := 0; i < len(d) && d[i] != '}' && d[i] != ']'; idx++ {
		if idx%jsonCheckpoint == 0 {
			cp = append(cp, i)
		}
		if obj {
			end, _ := jsonSkip(d, i)
			i = jsonWS(d, end)
			i = jsonWS(d, i+1)
		}
		end, _ := jsonSkip(d, i)
		i = jsonWS(d, end)
		if i < len(d) && d[i] == ',' {
			i = jsonWS(d, i+1)
		}
	}
	return cp
}

// CopyValue returns strings unquoted and everything else as its JSON text.
func (s *jsonSource) CopyValue(n *ValueNode) string {
	off, ok := n.Ref.(int)
	if !ok || off >= len(s.data) {
		return n.Value
	}
	end, _ := jsonSkip(s.data, off)
	if n.Kind == ValueString {
		return jsonString(s.data[off:end])
	}
	return string(s.data[off:end])
}

func jsonWS(d []byte, i int) int {
	for i < len(d) && (d[i] == ' ' || d[i] == '\n' || d[i] == '\t' || d[i] == '\r') {
		i++
	}
	return i
}

// jsonSkip returns the end of the (valid) value at i and, for objects and
// arrays, the number of their members.
func jsonSkip(d []byte, i int) (end, count int) {
	if i >= len(d) {
		return i, 0
	}
	switch d[i] {
	case '"':
		i++
		for i < len(d) {
			switch d[i] {
			case '\\':
				i += 2
				continue
			case '"':
				return i + 1, 0
			}
			i++
		}
		return i, 0
	case '{', '[':
		depth := 0
		inStr := false
		empty := true
		for ; i < len(d); i++ {
			c := d[i]
			if inStr {
				switch c {
				case '\\':
					i++
				case '"':
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
				if depth == 1 {
					empty = false
				}
			case '{', '[':
				depth++
				if depth == 2 {
					empty = false
				}
			case '}', ']':
				depth--
				if depth == 0 {
					if !empty {
						count++
					}
					return i + 1, count
				}
			case ',':
				if depth == 1 {
					count++
				}
			case ' ', '\n', '\t', '\r', ':':
			default:
				if depth == 1 {
					empty = false
				}
			}
		}
		return i, count
	}
	for i < len(d) {
		switch d[i] {
		case ',', '}', ']', ' ', '\n', '\t', '\r':
			return i, 0
		}
		i++
	}
	return i, 0
}

// jsonString decodes a JSON string literal.
func jsonString(lit []byte) string {
	if len(lit) >= 2 && !strings.ContainsRune(string(lit), '\\') {
		return string(lit[1 : len(lit)-1])
	}
	var s string
	if json.Unmarshal(lit, &s) != nil {
		return strings.Trim(string(lit), `"`)
	}
	return s
}

// jsonShort cuts s to n runes with an ellipsis.
func jsonShort(s string, n int) string {
	if len(s) <= n {
		return s
	}
	k := 0
	for i := range s {
		if k == n {
			return s[:i] + "…"
		}
		k++
	}
	return s
}

// ---------------------------------------------------------------------------
// Go value source (reflection).

type goSource struct {
	v      any
	mu     sync.Mutex
	sorted map[uintptr][]reflect.Value // map pointer -> sorted keys
}

// GoValueSource returns a ValueSource over a Go value: structs (exported
// fields, named by their json tag when present), maps (sorted keys),
// slices, arrays, pointers and interfaces are expanded; values with a
// String or Error method that are not containers show that text.
func GoValueSource(v any) ValueSource { return &goSource{v: v, sorted: map[uintptr][]reflect.Value{}} }

func (s *goSource) Roots() []ValueNode {
	n := s.node(reflect.ValueOf(s.v))
	return []ValueNode{n}
}

func goDeref(v reflect.Value) reflect.Value {
	for i := 0; i < 64 && v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface); i++ {
		if v.IsNil() {
			return v
		}
		v = v.Elem()
	}
	return v
}

func (s *goSource) node(v reflect.Value) ValueNode {
	n := s.node1(v)
	if v.IsValid() {
		n.Type = v.Type().String()
	}
	return n
}

func (s *goSource) node1(v reflect.Value) ValueNode {
	n := ValueNode{Ref: v}
	if !v.IsValid() {
		n.Kind, n.Value = ValueNull, "null"
		return n
	}
	if v.CanInterface() && v.Kind() != reflect.Pointer && v.Kind() != reflect.Interface {
		switch x := v.Interface().(type) {
		case fmt.Stringer:
			if k := v.Kind(); k != reflect.Map && k != reflect.Slice && k != reflect.Array {
				n.Kind, n.Value = ValueString, jsonShort(x.String(), jsonValueMax)
				return n
			}
		case error:
			n.Kind, n.Value = ValueString, jsonShort(x.Error(), jsonValueMax)
			return n
		}
	}
	d := goDeref(v)
	if !d.IsValid() || ((d.Kind() == reflect.Pointer || d.Kind() == reflect.Interface || d.Kind() == reflect.Map || d.Kind() == reflect.Slice) && d.IsNil()) {
		n.Kind, n.Value = ValueNull, "null"
		return n
	}
	if d != v {
		n := s.node1(d)
		n.Ref = v
		return n
	}
	switch d.Kind() {
	case reflect.Bool:
		n.Kind, n.Value = ValueBool, strconv.FormatBool(d.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n.Kind, n.Value = ValueNumber, strconv.FormatInt(d.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n.Kind, n.Value = ValueNumber, strconv.FormatUint(d.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		n.Kind, n.Value = ValueNumber, strconv.FormatFloat(d.Float(), 'g', -1, d.Type().Bits())
	case reflect.Complex64, reflect.Complex128:
		n.Kind, n.Value = ValueNumber, fmt.Sprint(d.Complex())
	case reflect.String:
		n.Kind, n.Value = ValueString, jsonShort(d.String(), jsonValueMax)
	case reflect.Struct:
		n.Kind, n.Count = ValueObject, len(goFields(d.Type()))
	case reflect.Map:
		n.Kind, n.Count = ValueObject, d.Len()
	case reflect.Slice, reflect.Array:
		n.Kind, n.Count = ValueArray, d.Len()
	default:
		n.Kind, n.Value = ValueOther, d.Type().String()
	}
	return n
}

type goField struct {
	index int
	name  string
}

var goFieldCache sync.Map // reflect.Type -> []goField

func goFields(t reflect.Type) []goField {
	if f, ok := goFieldCache.Load(t); ok {
		return f.([]goField)
	}
	var out []goField
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if tag, ok := f.Tag.Lookup("json"); ok {
			tn, _, _ := strings.Cut(tag, ",")
			if tn == "-" {
				continue
			}
			if tn != "" {
				name = tn
			}
		}
		out = append(out, goField{i, name})
	}
	goFieldCache.Store(t, out)
	return out
}

func (s *goSource) Children(parent *ValueNode, from, to int) ([]ValueNode, bool) {
	v, ok := parent.Ref.(reflect.Value)
	if !ok {
		return nil, true
	}
	v = goDeref(v)
	if !v.IsValid() {
		return nil, true
	}
	if to < 0 || to > parent.Count {
		to = parent.Count
	}
	var out []ValueNode
	switch v.Kind() {
	case reflect.Struct:
		fs := goFields(v.Type())
		for i := from; i < to && i < len(fs); i++ {
			n := s.node(v.Field(fs[i].index))
			n.Name = fs[i].name
			out = append(out, n)
		}
	case reflect.Map:
		keys := s.mapKeys(v)
		for i := from; i < to && i < len(keys); i++ {
			n := s.node(v.MapIndex(keys[i]))
			n.Name = fmt.Sprint(keys[i].Interface())
			out = append(out, n)
		}
	case reflect.Slice, reflect.Array:
		for i := from; i < to && i < v.Len(); i++ {
			n := s.node(v.Index(i))
			n.Name, n.Index = strconv.Itoa(i), true
			out = append(out, n)
		}
	}
	return out, true
}

func (s *goSource) mapKeys(v reflect.Value) []reflect.Value {
	p := v.Pointer()
	s.mu.Lock()
	defer s.mu.Unlock()
	if ks, ok := s.sorted[p]; ok && len(ks) == v.Len() {
		return ks
	}
	ks := v.MapKeys()
	sort.Slice(ks, func(i, j int) bool {
		a, b := ks[i], ks[j]
		if a.Kind() == reflect.String && b.Kind() == reflect.String {
			return a.String() < b.String()
		}
		return fmt.Sprint(a.Interface()) < fmt.Sprint(b.Interface())
	})
	s.sorted[p] = ks
	return ks
}

// CopyValue returns strings as they are and everything else as JSON (or
// its Go syntax when it cannot be encoded).
func (s *goSource) CopyValue(n *ValueNode) string {
	v, ok := n.Ref.(reflect.Value)
	if !ok || !v.IsValid() {
		return n.Value
	}
	d := goDeref(v)
	if d.IsValid() && d.Kind() == reflect.String {
		return d.String()
	}
	if !v.CanInterface() {
		return n.Value
	}
	if b, err := json.MarshalIndent(v.Interface(), "", "  "); err == nil {
		return string(b)
	}
	return fmt.Sprintf("%#v", v.Interface())
}

// jsonQuote quotes a key for a path when it is not an identifier.
func jsonPathKey(k string) string {
	ok := k != ""
	for i, r := range k {
		if !(r == '_' || r == '$' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9') || r >= utf8.RuneSelf) {
			ok = false
			break
		}
	}
	if ok {
		return "." + k
	}
	return "[" + strconv.Quote(k) + "]"
}
