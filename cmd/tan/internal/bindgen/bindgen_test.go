package bindgen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const appSrc = `package main

import (
	"context"
	"time"

	tb "github.com/tangoui/tango/bind"
	"github.com/tangoui/tango/ipc"
)

type Todo struct {
	ID    int       ` + "`json:\"id\"`" + `
	Title string    ` + "`json:\"title\"`" + `
	Due   *time.Time ` + "`json:\"due,omitempty\"`" + `
}

type Todos struct{}

// List returns every todo.
//
// Done ones too.
func (*Todos) List(ctx context.Context) ([]Todo, error) { return nil, nil }

func (*Todos) Add(title string, tags ...string) (*Todo, error) { return nil, nil }

func (*Todos) Clear() error { return nil }

func (Todos) Count() int { return 0 }

func (*Todos) Rename(id int, new string) {}

func (*Todos) helper() {}

type Greeter struct{}

func (Greeter) Hello(name string) string { return "hi " + name }

const greeterName = "Greeter"

func main() {
	r := ipc.NewRouter()
	_ = tb.Register(r, "Todos", &Todos{})
	_ = tb.Register(r, greeterName, Greeter{})
	name := "dynamic"
	_ = tb.Register(r, name, Greeter{})
}
`

// writeApp writes a module using this Tango checkout.
func writeApp(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	sum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		t.Skip("not inside a Tango checkout")
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":  "module app\n\ngo 1.27\n\nrequire github.com/tangoui/tango v0.0.0\n\nreplace github.com/tangoui/tango => " + filepath.ToSlash(root) + "\n",
		"go.sum":  string(sum),
		"main.go": appSrc,
	}
	for n, s := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
	return dir
}

func TestAnalyzeAndGenerate(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks from source")
	}
	res, err := Analyze(writeApp(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Services) != 2 {
		t.Fatalf("services %+v, warnings %v", res.Services, res.Warnings)
	}
	var dyn bool
	for _, w := range res.Warnings {
		if strings.Contains(w, "not a constant string") {
			dyn = true
		} else {
			t.Errorf("unexpected warning: %s", w)
		}
	}
	if !dyn {
		t.Error("no warning for the dynamic name")
	}
	files, err := Generate(res, "@tangoui/runtime")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"Todos.ts": Header + `import { call } from "@tangoui/runtime";
import type { Todo } from "./models";

export function Add(title: string, ...tags: string[]): Promise<Todo | null> {
  return call<Todo | null>("Todos.Add", title, ...tags);
}

export function Clear(): Promise<void> {
  return call<void>("Todos.Clear");
}

export function Count(): Promise<number> {
  return call<number>("Todos.Count");
}

/**
 * List returns every todo.
 *
 * Done ones too.
 */
export function List(): Promise<Todo[]> {
  return call<Todo[]>("Todos.List");
}

export function Rename(id: number, new_: string): Promise<void> {
  return call<void>("Todos.Rename", id, new_);
}
`,
		"Greeter.ts": Header + `import { call } from "@tangoui/runtime";

export function Hello(name: string): Promise<string> {
  return call<string>("Greeter.Hello", name);
}
`,
		"models.ts": Header + `
export interface Todo {
  id: number;
  title: string;
  due?: string | null;
}
`,
		"index.ts": Header + `export * as Todos from "./Todos";
export * as Greeter from "./Greeter";
export type * from "./models";
`,
	}
	if len(files) != len(want) {
		t.Errorf("files %d, want %d", len(files), len(want))
	}
	for n, w := range want {
		if got := string(files[n]); got != w {
			t.Errorf("%s:\n%s\nwant:\n%s", n, got, w)
		}
	}
}

func TestGenerateNoServices(t *testing.T) {
	if _, err := Generate(&Result{}, "x"); err != ErrNoServices {
		t.Errorf("err = %v", err)
	}
}
