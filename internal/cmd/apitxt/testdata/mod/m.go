// Package m is the root package of the apitxt test module.
package m

import (
	"example.com/m/internal/impl"
	"example.com/m/ui"
)

// Event and Kind are aliases of internal types: the golden file expands
// Event's fields and methods.
type (
	Event = impl.Event
	Kind  = impl.Kind
)

// Kinds.
const (
	KindNone Kind = iota
	KindOpen
)

// Placement is an enum.
type Placement uint8

// Placements.
const (
	PlaceRight Placement = iota
	PlaceLeft
	PlaceTop = PlaceLeft + 2
)

// Untyped constants.
const (
	Name  = "m"
	Ratio = 1.5
	Big   = 1 << 40
)

// Options is not comparable because of the slice field.
type Options struct {
	Title string `json:"title"`
	Tags  []string
	note  string
}

// Point is comparable.
type Point struct{ X, Y int }

// Sealed has an unexported method.
type Sealed interface {
	Do(int) (string, error)
	sealed()
}

// App embeds an internal type: its promoted fields and methods are listed.
type App struct {
	impl.Base
	name string
}

// Default is a package variable.
var Default *App

// New creates an App.
func New(o Options, extra ...string) *App { return nil }

// Name returns the name.
func (a *App) Name() string { return a.name }

// Keys is generic.
func Keys[K comparable, V any](m map[K]V) []K { return nil }

// UseUI leaks a non-allowlisted ui type into the stable root package.
func UseUI(c *ui.Context) {}
