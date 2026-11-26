package platform

import "github.com/tangoui/tango/ui/input"

// WebKeySink dispatches a key chord in a development build. Called on the
// main thread. done fires once, once native dispatch has completed.
type WebKeySink interface {
	DispatchWebKey(key input.Key, mods input.Modifiers, done func(error))
}

type WebEvaluator interface {
	EvalWebJSON(expression string, done func([]byte, error))
}

type WebInputKind uint8

const (
	WebClick WebInputKind = iota + 1
	WebType
	WebScroll
)

type WebInput struct {
	Kind           WebInputKind
	X, Y           float64
	DeltaX, DeltaY float64
	Text           string
}

type WebInputSink interface {
	DispatchWebInput(in WebInput, done func(error))
}

// WebJSONExpression wraps a synchronous expression in a JSON conversion
// for native APIs that return JavaScript values.
func WebJSONExpression(expression string) string {
	return "JSON.stringify((\n" + expression + "\n) ?? null)"
}
