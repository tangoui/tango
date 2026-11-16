package bind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type customArgument struct{ Text string }

func (v *customArgument) UnmarshalJSON(data []byte) error {
	if string(data) == `"reject"` {
		return errors.New("custom rejection")
	}
	return json.Unmarshal(data, &v.Text)
}

func TestBoundArgumentsPreserveLegacyBehavior(t *testing.T) {
	values := []string{`0`, `7`, `-3`, `1.25`, `true`, `null`, `"-"`, `"a"`, `"中\\文"`, `[]`, `[1,2]`, `{"X":2,"Y":4}`, `{"a":1}`, `"reject"`, `"\u00e9"`}
	inputs := []string{`[]`, `null`, ` [ ] `, "\r\n null\t", ``, ` `, `{}`, `1`, `"[]"`, `[`, `[1,]`, `[1]x`, `[null`, `["\q"]`, "\u00a0[]", `["a","b"]`}
	var seed uint32 = 4129
	for n := 0; n < 1000; n++ {
		parts := make([]string, n%6)
		for i := range parts {
			seed = seed*1664525 + 1013904223
			parts[i] = values[int(seed>>8)%len(values)]
		}
		s := "[" + strings.Join(parts, ", ") + "]"
		if n%3 == 0 {
			s = " \n" + s + "\t"
		}
		if n%11 == 0 {
			s += " false"
		}
		inputs = append(inputs, s)
	}
	for _, fn := range []any{
		func() int { return 7 },
		func(ctx context.Context) any { return ctx.Value("offset") },
		func(a, b int) (int, error) {
			if b < 0 {
				return 0, errors.New("negative")
			}
			return a + b, nil
		},
		func(ctx context.Context, p Point) Point { return Point{p.Y, p.X + ctx.Value("offset").(int)} },
		func(sep string, parts ...string) string { return strings.Join(parts, sep) },
		func(ctx context.Context, nums ...int) int {
			n := ctx.Value("offset").(int)
			for _, v := range nums {
				n += v
			}
			return n
		},
		func(v any) any { return v },
		func(v *Point) *Point { return v },
		func(v []int, m map[string]int) []int { return append(v, m["a"]) },
		func(v customArgument) customArgument { return v },
		func(v json.RawMessage) json.RawMessage { return v },
	} {
		v := reflect.ValueOf(fn)
		calls := 0
		// Count actual service invocations while still executing the real fn.
		tracked := reflect.MakeFunc(v.Type(), func(in []reflect.Value) []reflect.Value {
			calls++
			if v.Type().IsVariadic() {
				return v.CallSlice(in)
			}
			return v.Call(in)
		})
		current, err := makeHandler(tracked)
		if err != nil {
			t.Fatal(err)
		}
		before, err := referenceMakeHandler(tracked)
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.WithValue(context.Background(), "offset", 3)
		for _, args := range inputs {
			calls = 0
			got, gotErr := current(ctx, json.RawMessage(args))
			gotCalls := calls
			calls = 0
			want, wantErr := before(ctx, json.RawMessage(args))
			if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotErr, wantErr) || gotCalls != calls {
				t.Fatalf("%s(%q): got %#v/%v/%d calls, want %#v/%v/%d calls", v.Type(), args, got, gotErr, gotCalls, want, wantErr, calls)
			}
		}
	}
}

func TestBoundArgumentsKeepIndependentResults(t *testing.T) {
	h, err := makeHandler(reflect.ValueOf(func(v json.RawMessage) json.RawMessage { return v }))
	if err != nil {
		t.Fatal(err)
	}
	args := json.RawMessage(`[{"n":1}]`)
	first, err := h(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h(context.Background(), json.RawMessage(`[{"n":2}]`)); err != nil {
		t.Fatal(err)
	}
	for i := range args {
		args[i] = 'x'
	}
	if got := string(first.(json.RawMessage)); got != `{"n":1}` {
		t.Fatalf("earlier result changed: %q", got)
	}
}

func TestBoundArgumentsConcurrentContext(t *testing.T) {
	h, err := makeHandler(reflect.ValueOf(func(ctx context.Context, n int) int { return ctx.Value("offset").(int) + n }))
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for worker := range 16 {
		wg.Go(func() {
			ctx := context.WithValue(context.Background(), "offset", worker*1000)
			for n := range 64 {
				got, err := h(ctx, json.RawMessage(fmt.Sprintf("[%d]", n)))
				if err != nil || got != worker*1000+n {
					t.Errorf("worker %d argument %d: %v, %v", worker, n, got, err)
					return
				}
			}
		})
	}
	wg.Wait()
}
