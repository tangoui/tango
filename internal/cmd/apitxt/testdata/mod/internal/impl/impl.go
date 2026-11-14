// Package impl is internal.
package impl

// Kind is an event kind.
type Kind int

// Event is aliased by the root package.
type Event struct {
	Kind   Kind
	URL    string
	secret int
}

// String formats the event.
func (e Event) String() string { return e.URL }

// Base is embedded by m.App.
type Base struct {
	ID int
}

// Close closes it.
func (b *Base) Close() error { return nil }
