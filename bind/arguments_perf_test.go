package bind

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func BenchmarkBoundArguments(b *testing.B) {
	type record struct {
		Name string
		Data []int
	}
	for _, tc := range []struct {
		name, args string
		fn         any
		want       any
	}{
		{"context-whitespace", " \t[ \n ]\r", (&svc{}).CtxOnly, "ctx"},
		{"context-value", `[7]`, func(ctx context.Context, n int) int { return n + ctx.Value("offset").(int) }, 10},
		{"large-string", `["` + strings.Repeat("a", 64<<10) + `"]`, func(s string) int { return len(s) }, 64 << 10},
		{"nested", `[{"Name":"abc","Data":[1,2,3,4,5]}]`, func(r record) int { return len(r.Name) + len(r.Data) }, 8},
	} {
		b.Run(tc.name, func(b *testing.B) {
			h, err := makeHandler(reflect.ValueOf(tc.fn))
			if err != nil {
				b.Fatal(err)
			}
			args := json.RawMessage(tc.args)
			ctx := context.WithValue(context.Background(), "offset", 3)
			b.ReportAllocs()
			for b.Loop() {
				got, err := h(ctx, args)
				if err != nil || got != tc.want {
					b.Fatal("bad return", got, err)
				}
			}
		})
	}
}
