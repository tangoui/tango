//go:build !race

package bind

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestBoundCallAllocationBudget(t *testing.T) {
	s := &svc{}
	for _, tc := range []struct {
		name, args string
		fn         any
		want       any
		max        float64
	}{
		{"add", `[2,3]`, s.Add, 5, 10},
		{"context", `[]`, s.CtxOnly, "ctx", 3},
		{"context-null", `null`, s.CtxOnly, "ctx", 3},
		{"context-whitespace", " \t[ \n ]\r", s.CtxOnly, "ctx", 4},
		{"context-value", `[7]`, func(ctx context.Context, n int) int { return n + ctx.Value("offset").(int) }, 10, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := makeHandler(reflect.ValueOf(tc.fn))
			if err != nil {
				t.Fatal(err)
			}
			args := json.RawMessage(tc.args)
			ctx := context.WithValue(context.Background(), "offset", 3)
			allocs := testing.AllocsPerRun(100, func() {
				got, err := h(ctx, args)
				if err != nil || got != tc.want {
					panic("wrong bound result")
				}
			})
			if allocs > tc.max {
				t.Fatalf("call allocates %.0f times, budget %.0f", allocs, tc.max)
			}
		})
	}
}
