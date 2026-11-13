package editor

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui/editor/when"
)

// Command is an editor command: something a keybinding, a menu or the
// command palette runs on a State by ID (State.Execute). IDs are Monaco's
// ("cursorWordLeft", "editor.action.selectAll").
type Command struct {
	// ID is the command's identifier, Monaco's command id where Monaco has
	// the command.
	ID string
	// Label is the title shown in the command palette and menus.
	Label string
	// Precondition is a when expression (package when) over the editor's
	// context keys (see contextkeys.go) that must hold for the command to
	// run; empty means always.
	Precondition string
	// Mutates marks a command that edits the text: it is refused while the
	// view is read-only.
	Mutates bool
	// Run runs the command with the arguments of the keybinding rule (or
	// the caller of Execute) and reports whether it did something.
	Run func(st *State, args any) bool
}

type regCommand struct {
	Command
	pre when.Expr // parsed Precondition, nil for none
}

var commands = struct {
	sync.RWMutex
	m map[string]*regCommand
}{m: map[string]*regCommand{}}

// RegisterCommand adds cmd to the global command table, replacing a
// command with the same ID (as VS Code's CommandsRegistry: the last
// registration wins). It panics when the ID or Run is missing or the
// Precondition does not parse: those are programming errors.
func RegisterCommand(cmd Command) {
	if cmd.ID == "" || cmd.Run == nil {
		panic("editor: RegisterCommand needs an ID and Run")
	}
	rc := &regCommand{Command: cmd}
	if strings.TrimSpace(cmd.Precondition) != "" {
		e, err := when.Parse(cmd.Precondition)
		if err != nil {
			panic(fmt.Sprintf("editor: command %s: precondition: %v", cmd.ID, err))
		}
		rc.pre = e
	}
	commands.Lock()
	commands.m[cmd.ID] = rc
	commands.Unlock()
}

// Commands returns the registered commands sorted by ID (for the command
// palette).
func Commands() []Command {
	commands.RLock()
	out := make([]Command, 0, len(commands.m))
	for _, c := range commands.m {
		out = append(out, c.Command)
	}
	commands.RUnlock()
	slices.SortFunc(out, func(a, b Command) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// LookupCommand returns the registered command id.
func LookupCommand(id string) (Command, bool) {
	commands.RLock()
	c, ok := commands.m[id]
	commands.RUnlock()
	if !ok {
		return Command{}, false
	}
	return c.Command, true
}

// Execute runs the registered command id with args on the State and
// reports whether it ran and did something. It refuses unknown commands,
// commands whose Precondition does not hold (evaluated over the context
// keys of the view that last showed the State, see contextkeys.go) and
// Mutates commands while the view is read-only. Commands that need the
// view (vertical moves by layout x, the clipboard, revealing the caret
// horizontally) use the view that last showed the State; without one they
// fall back to the State's own column model, and clipboard commands do
// nothing.
func (s *State) Execute(id string, args any) bool {
	return s.execute(id, args, s.contextKeys())
}

// execute is Execute with the Precondition evaluated in ctx (a menu of the
// view runs its commands as if the view had focus).
func (s *State) execute(id string, args any, ctx when.Context) bool {
	commands.RLock()
	c, ok := commands.m[id]
	commands.RUnlock()
	if !ok {
		return false
	}
	if c.pre != nil && !c.pre.Eval(ctx) {
		return false
	}
	if c.Mutates && s.readOnly() {
		return false
	}
	s.gate = nil // each command gets its own synchronous tokenization budget
	return c.Run(s, args)
}

// readOnly reports whether edits are refused: the last View's ReadOnly
// for this frame, else Options.ReadOnly.
func (s *State) readOnly() bool {
	if s.view != nil {
		return s.view.opts.ReadOnly
	}
	return s.Options.ReadOnly
}
