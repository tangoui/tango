package main

import (
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// TestConfigSchema keeps tan.schema.json, the JSON Schema of tan.json that
// editors use for completion (published at SchemaURL), in step with Config:
// the properties come from the json tags, the descriptions from the field
// comments. Run "go test -run TestConfigSchema -update" after changing
// Config. Fields are only added, never removed; a renamed field keeps
// parsing under its old name (docs/release.md).
func TestConfigSchema(t *testing.T) {
	got := configSchema(t)
	const path = "tan.schema.json"
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -run TestConfigSchema -update)", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(want, got) {
		t.Errorf("tan.schema.json is stale: run go test -run TestConfigSchema -update and review the diff\n%s", lineDiff(string(want), string(got)))
	}
}

// TestConfigSchemaParses checks that a tan.json with "$schema" parses.
func TestConfigSchemaParses(t *testing.T) {
	c, err := ParseConfig([]byte(`{"$schema": "` + SchemaURL + `", "name": "Hello", "id": "com.example.hello", "version": "1.0.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	if c.Schema != SchemaURL {
		t.Fatalf("$schema: %q", c.Schema)
	}
	_, err = ParseConfig([]byte(`{"name": "Hello", "id": "com.example.hello", "version": "1.0.0", "fromTheFuture": 1}`))
	if err == nil || !strings.Contains(err.Error(), "newer tan") {
		t.Fatalf("unknown field: %v", err)
	}
}

// schemaEnums are the closed value sets validate enforces.
var schemaEnums = map[string][]string{
	"FileAssociation.Role":          {"editor", "viewer"},
	"NSISInstaller.Scope":           {"user", "machine"},
	"NSISInstaller.DesktopShortcut": {"ask", "always", "never"},
}

func configSchema(t *testing.T) []byte {
	t.Helper()
	docs := fieldDocs(t)
	defs := map[string]any{}
	var typeSchema func(rt reflect.Type, key string) map[string]any
	structRef := func(rt reflect.Type) map[string]any {
		name := rt.Name()
		if _, ok := defs[name]; !ok {
			defs[name] = nil // reserve against recursion
			props := map[string]any{}
			var required []string
			for f := range rt.Fields() {
				tag := f.Tag.Get("json")
				if !f.IsExported() || tag == "-" {
					continue
				}
				jname, opts, _ := strings.Cut(tag, ",")
				if jname == "" {
					jname = f.Name
				}
				key := name + "." + f.Name
				s := typeSchema(f.Type, key)
				if d := docs[key]; d != "" {
					s["description"] = d
				}
				props[jname] = s
				if !strings.Contains(opts, "omit") {
					required = append(required, jname)
				}
			}
			d := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
			if name == "Config" {
				// The rest of validate: frontend is optional (self-drawn apps).
				required = []string{"name", "id", "version"}
			}
			if len(required) > 0 {
				d["required"] = required
			}
			if doc := docs[name]; doc != "" {
				d["description"] = doc
			}
			defs[name] = d
		}
		return map[string]any{"$ref": "#/$defs/" + name}
	}
	typeSchema = func(rt reflect.Type, key string) map[string]any {
		if rt.Kind() == reflect.Pointer {
			rt = rt.Elem()
		}
		switch rt.Kind() {
		case reflect.String:
			s := map[string]any{"type": "string"}
			if e, ok := schemaEnums[key]; ok {
				s["enum"] = e
			}
			return s
		case reflect.Bool:
			return map[string]any{"type": "boolean"}
		case reflect.Int, reflect.Int64:
			return map[string]any{"type": "integer"}
		case reflect.Slice:
			s := map[string]any{"type": "array", "items": typeSchema(rt.Elem(), key+"[]")}
			if key == "Config.Permissions" {
				var names []string
				for _, p := range permissions {
					names = append(names, p.name)
				}
				slices.Sort(names)
				s["items"] = map[string]any{"type": "string", "enum": names}
			}
			return s
		case reflect.Map:
			return map[string]any{"type": "object", "additionalProperties": typeSchema(rt.Elem(), key+"{}")}
		case reflect.Struct:
			return structRef(rt)
		}
		t.Fatalf("%s: no schema for %s", key, rt)
		return nil
	}
	structRef(reflect.TypeFor[Config]())
	cfg := defs["Config"].(map[string]any)
	delete(defs, "Config")
	out := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     SchemaURL,
		"title":   "tan.json",
	}
	for k, v := range cfg {
		out[k] = v
	}
	out["$defs"] = defs
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// fieldDocs maps "Type.Field" and "Type" to their comments in the
// package's non-test sources.
func fieldDocs(t *testing.T) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{}
	clean := func(groups ...*ast.CommentGroup) string {
		for _, g := range groups {
			if g == nil {
				continue
			}
			// Prose only: indented lines are code examples.
			var prose []string
			for _, l := range strings.Split(g.Text(), "\n") {
				if !strings.HasPrefix(l, "\t") && !strings.HasPrefix(l, " ") {
					prose = append(prose, l)
				}
			}
			s := strings.Join(strings.Fields(strings.Join(prose, " ")), " ")
			if s != "" {
				return s
			}
		}
		return ""
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, n, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, sp := range g.Specs {
				ts := sp.(*ast.TypeSpec)
				st, ok := ts.Type.(*ast.StructType)
				if !ok || !ast.IsExported(ts.Name.Name) {
					continue
				}
				if s := clean(ts.Doc, g.Doc); s != "" && !strings.HasPrefix(s, "\"") {
					docs[ts.Name.Name] = s
				}
				for _, fl := range st.Fields.List {
					for _, id := range fl.Names {
						if s := clean(fl.Doc, fl.Comment); s != "" {
							docs[ts.Name.Name+"."+id.Name] = s
						}
					}
				}
			}
		}
	}
	return docs
}
