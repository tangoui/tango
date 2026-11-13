package bindgen

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

const mapperSrc = `package p

import (
	"encoding/json"
	"net/netip"
	"time"
)

type Color string

type Base struct {
	ID      int64     ` + "`json:\"id\"`" + `
	Created time.Time ` + "`json:\"created\"`" + `
}

type User struct {
	Base
	Name     string            ` + "`json:\"name\"`" + `
	Email    *string           ` + "`json:\"email\"`" + `
	Tags     []string          ` + "`json:\"tags,omitempty\"`" + `
	Avatar   []byte            ` + "`json:\"avatar\"`" + `
	Scores   map[string]float64
	Friends  []*User           ` + "`json:\"friends\"`" + `
	Fav      Color             ` + "`json:\"fav\"`" + `
	Big      int64             ` + "`json:\"big,string\"`" + `
	Extra    json.RawMessage   ` + "`json:\"extra\"`" + `
	Addr     netip.Addr        ` + "`json:\"addr\"`" + `
	Secret   string            ` + "`json:\"-\"`" + `
	private  int
	Point    struct{ X, Y int } ` + "`json:\"point\"`" + `
	Any      any               ` + "`json:\"any\"`" + `
	Grid     [2][3]uint8       ` + "`json:\"grid\"`" + `
	Ch       chan int          ` + "`json:\"-\"`" + `
	ByID     map[int]Base      ` + "`json:\"by_id\"`" + `
	Maybe    []*int            ` + "`json:\"maybe\"`" + `
}

type Page[T any] struct {
	Items []T ` + "`json:\"items\"`" + `
	Next  *string ` + "`json:\"next,omitzero\"`" + `
}

var (
	vBool    bool
	vInt     int
	vUint8   uint8
	vFloat   float32
	vStr     string
	vBytes   []byte
	vTime    time.Time
	vPtr     *int
	vSlice   []string
	vMap     map[string][]int
	vUser    User
	vUserPtr *User
	vPage    Page[User]
	vIface   interface{ M() }
	vFunc    func()
	vColor   Color
	vErr     error
	vDur     time.Duration
	vNumber  json.Number
)
`

func checkSrc(t *testing.T) *types.Package {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", mapperSrc, 0)
	if err != nil {
		t.Fatal(err)
	}
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil)}
	pkg, err := conf.Check("p", fset, []*ast.File{f}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return pkg
}

func TestMapperTypes(t *testing.T) {
	pkg := checkSrc(t)
	m := NewMapper()
	for name, want := range map[string]string{
		"vBool":    "boolean",
		"vInt":     "number",
		"vUint8":   "number",
		"vFloat":   "number",
		"vStr":     "string",
		"vBytes":   "string",
		"vTime":    "string",
		"vPtr":     "number | null",
		"vSlice":   "string[]",
		"vMap":     "Record<string, number[]>",
		"vUser":    "User",
		"vUserPtr": "User | null",
		"vPage":    "Page_User",
		"vIface":   "any",
		"vFunc":    "unknown",
		"vColor":   "string",
		"vErr":     "any",
		"vDur":     "number",
		"vNumber":  "number",
	} {
		got := m.TS(pkg.Scope().Lookup(name).Type())
		if got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}
}

func TestMapperModels(t *testing.T) {
	pkg := checkSrc(t)
	m := NewMapper()
	m.TS(pkg.Scope().Lookup("vUser").Type())
	m.TS(pkg.Scope().Lookup("vPage").Type())
	got := modelsFile(m.Models())
	want := Header + `
export interface User {
  id: number;
  created: string;
  name: string;
  email?: string | null;
  tags?: string[];
  avatar: string;
  Scores: Record<string, number>;
  friends: (User | null)[];
  fav: string;
  big: string;
  extra: any;
  addr: string;
  point: { X: number; Y: number };
  any: any;
  grid: number[][];
  by_id: Record<string, Base>;
  maybe: (number | null)[];
}

export interface Base {
  id: number;
  created: string;
}

export interface Page_User {
  items: User[];
  next?: string | null;
}
`
	if got != want {
		t.Errorf("models.ts:\n%s\nwant:\n%s", got, want)
	}
}

func TestMapperNameClash(t *testing.T) {
	a := types.NewPackage("example.com/a", "a")
	b := types.NewPackage("example.com/b", "b")
	mk := func(p *types.Package) types.Type {
		st := types.NewStruct([]*types.Var{types.NewField(token.NoPos, p, "X", types.Typ[types.Int], false)}, nil)
		return types.NewNamed(types.NewTypeName(token.NoPos, p, "Item", nil), st, nil)
	}
	m := NewMapper()
	if got := m.TS(mk(a)); got != "Item" {
		t.Errorf("first = %q", got)
	}
	if got := m.TS(mk(b)); got != "BItem" {
		t.Errorf("clash = %q", got)
	}
	if !strings.Contains(modelsFile(m.Models()), "export interface BItem") {
		t.Error("BItem not generated")
	}
}
