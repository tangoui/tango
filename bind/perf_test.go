package bind

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func BenchmarkBoundCall(b *testing.B) {
	s := &svc{}
	for _, c := range []struct {
		name string
		fn   any
		args string
		want any
	}{
		{"add", s.Add, `[2,3]`, 5},
		{"context", s.CtxOnly, `[]`, "ctx"},
		{"variadic", s.Join, `["-","a","b","c"]`, "a-b-c"},
	} {
		b.Run(c.name, func(b *testing.B) {
			h, err := makeHandler(reflect.ValueOf(c.fn))
			if err != nil {
				b.Fatal(err)
			}
			args := json.RawMessage(c.args)
			ctx := context.Background()
			b.ReportAllocs()
			for b.Loop() {
				got, err := h(ctx, args)
				if err != nil || got != c.want {
					b.Fatal("bad return", got, err)
				}
			}
		})
	}
}
