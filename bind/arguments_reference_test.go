package bind

// Reference implementation from 902c52d; compare behavior, errors and ownership.
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tangoui/tango/ipc"
	"reflect"
)

func referenceMakeHandler(fn reflect.Value) (ipc.HandlerFunc, error) {
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

	return func(ctx context.Context, args json.RawMessage) (any, error) {
		in, err := referenceDecodeArgs(args, params, variadic)
		if err != nil {
			return nil, ipc.NewError(ipc.CodeBadArgs, err.Error())
		}
		if hasCtx {
			in = append([]reflect.Value{reflect.ValueOf(ctx)}, in...)
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

func referenceDecodeArgs(args json.RawMessage, params []reflect.Type, variadic bool) ([]reflect.Value, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(args, &raw); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON array: %v", err)
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
	in := make([]reflect.Value, 0, len(raw))
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
