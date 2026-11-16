// Package bind exposes the exported methods of a Go value as ipc calls named
// "Name.Method" using reflection.
//
// Supported method shapes: an optional leading context.Context, then any
// number of JSON-decodable parameters (a variadic final parameter is
// allowed), returning nothing, (T), (error) or (T, error). JS passes the
// parameters positionally: tango.call("Greeter.Hello", "world").
package bind

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/tangoui/tango/ipc"
)

var (
	ctxType = reflect.TypeFor[context.Context]()
	errType = reflect.TypeFor[error]()
)

// Register binds every exported method of svc under name. It fails, binding
// nothing, if any exported method has an unsupported signature or svc has no
// exported methods. Methods are taken from svc's method set, so pass a
// pointer to include pointer-receiver methods.
func Register(r *ipc.Router, name string, svc any) error {
	if r == nil || name == "" {
		return errors.New("bind: nil router or empty name")
	}
	if svc == nil {
		return errors.New("bind: nil service")
	}
	v := reflect.ValueOf(svc)
	t := v.Type()
	if t.NumMethod() == 0 {
		return fmt.Errorf("bind: %s has no exported methods", t)
	}
	handlers := make(map[string]ipc.HandlerFunc, t.NumMethod())
	for i := range t.NumMethod() {
		m := t.Method(i)
		h, err := makeHandler(v.Method(i))
		if err != nil {
			return fmt.Errorf("bind: %s.%s: %w", t, m.Name, err)
		}
		handlers[name+"."+m.Name] = h
	}
	for n, h := range handlers {
		r.Handle(n, h)
	}
	return nil
}

func makeHandler(fn reflect.Value) (ipc.HandlerFunc, error) {
	ft := fn.Type()
	hasCtx := ft.NumIn() > 0 && ft.In(0) == ctxType
	first := 0
	if hasCtx {
		first = 1
	}
	params := make([]reflect.Type, 0, ft.NumIn()-first)
	for i := first; i < ft.NumIn(); i++ {
		pt := ft.In(i)
		if pt == ctxType {
			return nil, errors.New("context.Context must be the first parameter")
		}
		check := pt
		if ft.IsVariadic() && i == ft.NumIn()-1 {
			check = pt.Elem()
		}
		if !jsonable(check) {
			return nil, fmt.Errorf("parameter %d type %s is not JSON-decodable", i, pt)
		}
		params = append(params, pt)
	}
	variadic := ft.IsVariadic()

	hasResult, hasErr := false, false
	switch ft.NumOut() {
	case 0:
	case 1:
		if ft.Out(0) == errType {
			hasErr = true
		} else {
			hasResult = true
		}
	case 2:
		if ft.Out(1) != errType || ft.Out(0) == errType {
			return nil, errors.New("two results must be (T, error)")
		}
		hasResult, hasErr = true, true
	default:
		return nil, errors.New("too many results")
	}
	if hasResult && !jsonable(ft.Out(0)) {
		return nil, fmt.Errorf("result type %s is not JSON-encodable", ft.Out(0))
	}
	noRequiredArgs := len(params) == 0 || (variadic && len(params) == 1)

	return func(ctx context.Context, args json.RawMessage) (any, error) {
		var prefix []reflect.Value
		if hasCtx {
			prefix = []reflect.Value{reflect.ValueOf(ctx)}
		}
		in := prefix
		// Keep empty calls in this frame so the Context-only slice can stay
		// on the stack. Other input still receives full JSON/arity checking.
		if !noRequiredArgs || (string(args) != "[]" && string(args) != "null") {
			var err error
			in, err = decodeArgs(args, params, variadic, prefix...)
			if err != nil {
				return nil, ipc.NewError(ipc.CodeBadArgs, err.Error())
			}
			if len(in) == 0 {
				in = prefix
			}
		}
		outs := fn.Call(in) // variadic values are already expanded in in
		if hasErr {
			if e := outs[len(outs)-1]; !e.IsNil() {
				return nil, e.Interface().(error)
			}
		}
		if hasResult {
			return outs[0].Interface(), nil
		}
		return nil, nil
	}, nil
}

func decodeArgs(args json.RawMessage, params []reflect.Type, variadic bool, prefix ...reflect.Value) ([]reflect.Value, error) {
	raw, err := rawArguments(args, len(params))
	if err != nil {
		return nil, err
	}
	fixed := len(params)
	if variadic {
		fixed--
		if len(raw) < fixed {
			return nil, fmt.Errorf("want at least %d arguments, got %d", fixed, len(raw))
		}
	} else if len(raw) != fixed {
		return nil, fmt.Errorf("want %d arguments, got %d", fixed, len(raw))
	}
	// Empty JSON with whitespace still went through the decoder. Let the
	// caller keep its stack-backed Context prefix rather than allocating it.
	if len(raw) == 0 {
		return nil, nil
	}
	// Include Context in the same allocation as the decoded parameters.
	in := make([]reflect.Value, 0, len(prefix)+len(raw))
	in = append(in, prefix...)
	for i, r := range raw {
		var pt reflect.Type
		if i < fixed {
			pt = params[i]
		} else {
			pt = params[fixed].Elem()
		}
		p := reflect.New(pt)
		if err := json.Unmarshal(r, p.Interface()); err != nil {
			return nil, fmt.Errorf("argument %d: %v", i, err)
		}
		in = append(in, p.Elem())
	}
	return in, nil
}

func rawArguments(args json.RawMessage, capacity int) ([]json.RawMessage, error) {
	// Return before declaring raw: its address escapes through Unmarshal.
	// Only these exact encodings skip decoding; whitespace and all other
	// input still pass through encoding/json.
	if string(args) == "[]" || string(args) == "null" {
		return nil, nil
	}
	raw := make([]json.RawMessage, 0, capacity)
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON array: %v", err)
	}
	return raw, nil
}

// jsonable rejects types encoding/json can never handle.
func jsonable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer, reflect.Complex64, reflect.Complex128:
		return false
	}
	return true
}
