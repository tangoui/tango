package bindgen

import (
	"go/types"
	"reflect"
	"strings"
)

// Model is a TypeScript interface generated for a named Go struct.
type Model struct {
	Name   string
	Fields []Field
}

// Field is one property of a Model (or of an inline object type).
type Field struct {
	Name     string
	Type     string
	Optional bool // pointer, omitempty or omitzero: may be absent or null
}

// Mapper turns Go types into TypeScript types the way encoding/json
// encodes them:
//
//   - bool → boolean; integers and floats → number; string → string
//   - []byte → string (base64); other slices and arrays → T[]
//   - map[K]V → Record<string, V> (JSON object keys are strings)
//   - *T → T | null; as a struct field it is also optional (name?:)
//   - named struct → an interface (Model) named after the Go type, fields
//     following json tags ("-" skipped, renamed, omitempty → optional,
//     ",string" → string) and embedded structs flattened
//   - anonymous struct → an inline object type
//   - time.Time and encoding.TextMarshaler types → string
//   - json.RawMessage, json.Marshaler types, interfaces → any
//   - error (as a result) is dropped by the caller; chans, funcs and
//     complex numbers cannot be encoded and map to unknown
//
// Named non-struct types map to their underlying type.
type Mapper struct {
	models map[string]*Model // by Go type string
	byName map[string]string // TS name → Go type string, to detect clashes
	order  []string          // Go type strings in first-use order
}

// NewMapper returns an empty Mapper.
func NewMapper() *Mapper {
	return &Mapper{models: map[string]*Model{}, byName: map[string]string{}}
}

// Models returns the interfaces generated so far, in first-use order.
func (m *Mapper) Models() []*Model {
	out := make([]*Model, 0, len(m.order))
	for _, k := range m.order {
		out = append(out, m.models[k])
	}
	return out
}

// TS returns the TypeScript type for t.
func (m *Mapper) TS(t types.Type) string {
	return m.ts(t, false)
}

func (m *Mapper) ts(t types.Type, asString bool) string {
	if t == nil {
		return "any"
	}
	if a, ok := t.(*types.Alias); ok {
		return m.ts(types.Unalias(a), asString)
	}
	if n, ok := t.(*types.Named); ok {
		if s, ok := special(n); ok {
			return s
		}
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return basic(u, asString)
	case *types.Pointer:
		return m.ts(u.Elem(), asString) + " | null"
	case *types.Slice:
		if b, ok := u.Elem().Underlying().(*types.Basic); ok && b.Kind() == types.Byte {
			if _, named := u.Elem().(*types.Named); !named {
				return "string"
			}
		}
		return arrayOf(m.ts(u.Elem(), false))
	case *types.Array:
		return arrayOf(m.ts(u.Elem(), false))
	case *types.Map:
		return "Record<string, " + m.ts(u.Elem(), false) + ">"
	case *types.Struct:
		if n, ok := t.(*types.Named); ok {
			return m.model(n, u)
		}
		return inline(m.fields(u))
	case *types.Interface:
		return "any"
	case *types.Chan, *types.Signature:
		return "unknown"
	}
	return "any"
}

// special handles named types whose JSON form is not their structure.
func special(n *types.Named) (string, bool) {
	obj := n.Obj()
	if obj.Pkg() != nil {
		switch obj.Pkg().Path() + "." + obj.Name() {
		case "time.Time":
			return "string", true
		case "encoding/json.RawMessage", "encoding/json.Number":
			if obj.Name() == "Number" {
				return "number", true
			}
			return "any", true
		}
	}
	if hasMethod(n, "MarshalJSON") {
		return "any", true
	}
	if hasMethod(n, "MarshalText") {
		return "string", true
	}
	return "", false
}

func hasMethod(t types.Type, name string) bool {
	for _, tt := range []types.Type{t, types.NewPointer(t)} {
		ms := types.NewMethodSet(tt)
		for i := range ms.Len() {
			if ms.At(i).Obj().Name() == name {
				return true
			}
		}
	}
	return false
}

func basic(b *types.Basic, asString bool) string {
	if asString {
		return "string"
	}
	switch {
	case b.Info()&types.IsBoolean != 0:
		return "boolean"
	case b.Info()&types.IsComplex != 0:
		return "unknown"
	case b.Info()&types.IsNumeric != 0:
		return "number"
	case b.Info()&types.IsString != 0:
		return "string"
	case b.Kind() == types.UntypedNil:
		return "null"
	}
	return "any"
}

func arrayOf(elem string) string {
	if strings.ContainsAny(elem, " |<{") && !strings.HasPrefix(elem, "Record<") {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
}

func inline(fs []Field) string {
	if len(fs) == 0 {
		return "Record<string, never>"
	}
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = f.decl()
	}
	return "{ " + strings.Join(parts, "; ") + " }"
}

func (f Field) decl() string {
	name := f.Name
	if !isIdent(name) {
		name = `"` + strings.ReplaceAll(name, `"`, `\"`) + `"`
	}
	if f.Optional {
		return name + "?: " + f.Type
	}
	return name + ": " + f.Type
}

func isIdent(s string) bool {
	for i, r := range s {
		if !(r == '_' || r == '$' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return s != ""
}

// model returns the interface name for a named struct, generating the
// interface the first time.
func (m *Mapper) model(n *types.Named, st *types.Struct) string {
	key := types.TypeString(n, nil)
	if mod, ok := m.models[key]; ok {
		return mod.Name
	}
	name := n.Obj().Name()
	if args := n.TypeArgs(); args != nil {
		for i := range args.Len() {
			name += "_" + typeArgName(args.At(i))
		}
	}
	if other, ok := m.byName[name]; ok && other != key && n.Obj().Pkg() != nil {
		name = exportName(n.Obj().Pkg().Name()) + name
	}
	mod := &Model{Name: name}
	m.models[key] = mod // before the fields, so recursive types terminate
	m.byName[name] = key
	m.order = append(m.order, key)
	mod.Fields = m.fields(st)
	return name
}

func typeArgName(t types.Type) string {
	s := types.TypeString(t, func(*types.Package) string { return "" })
	var b strings.Builder
	for _, r := range s {
		if r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return exportName(b.String())
}

func exportName(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// fields lists a struct's JSON properties.
func (m *Mapper) fields(st *types.Struct) []Field {
	var out []Field
	for i := range st.NumFields() {
		v := st.Field(i)
		tag := reflect.StructTag(st.Tag(i)).Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		if v.Embedded() && name == "" {
			et := v.Type()
			if p, ok := et.Underlying().(*types.Pointer); ok {
				et = p.Elem()
			}
			if est, ok := et.Underlying().(*types.Struct); ok {
				if _, isSpecial := specialType(et); !isSpecial {
					out = append(out, m.fields(est)...)
					continue
				}
			}
		}
		if !v.Exported() {
			continue
		}
		if name == "" {
			name = v.Name()
		}
		_, isPtr := v.Type().Underlying().(*types.Pointer)
		hasOpt := func(o string) bool {
			for _, x := range strings.Split(opts, ",") {
				if x == o {
					return true
				}
			}
			return false
		}
		f := Field{
			Name:     name,
			Type:     m.ts(v.Type(), hasOpt("string") && isScalar(v.Type())),
			Optional: isPtr || hasOpt("omitempty") || hasOpt("omitzero"),
		}
		out = append(out, f)
	}
	return out
}

// specialType is special for any type.
func specialType(t types.Type) (string, bool) {
	if n, ok := t.(*types.Named); ok {
		return special(n)
	}
	return "", false
}

// isScalar reports whether the ",string" tag option applies to t.
func isScalar(t types.Type) bool {
	if p, ok := t.Underlying().(*types.Pointer); ok {
		t = p.Elem()
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
}
