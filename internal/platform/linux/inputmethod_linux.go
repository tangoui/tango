//go:build linux && !android

package linux

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/platform"
)

var ibus struct {
	init                        func()
	busNew                      func() ptr
	busConnected                func(ptr) bool
	busConnection               func(ptr) ptr
	busRegister                 func(ptr, ptr) bool
	factoryNew                  func(ptr) ptr
	componentNew                func(string, string, string, string, string, string, string, string) ptr
	engineDescNew               func(string, string, string, string, string, string, string, string) ptr
	componentAdd                func(ptr, ptr)
	engineType                  func() ptr
	objectNew                   func(ptr, string, ...any) ptr
	destroy                     func(ptr)
	textNew                     func(string) ptr
	commit                      func(ptr, ptr)
	preedit                     func(ptr, ptr, uint32, int32, uint32)
	keyvalToUnicode             func(uint32) uint32
	create, key, in, out, caret ptr
}

var ibusHost *ibusServer
var ibusErr error

var loadIBus = sync.OnceFunc(func() {
	lib, _, err := open("libibus-1.0.so.5")
	if err != nil {
		ibusErr = err
		return
	}
	var bind binder
	for _, entry := range []struct {
		fn   any
		name string
	}{
		{&ibus.init, "ibus_init"},
		{&ibus.busNew, "ibus_bus_new"},
		{&ibus.busConnected, "ibus_bus_is_connected"},
		{&ibus.busConnection, "ibus_bus_get_connection"},
		{&ibus.busRegister, "ibus_bus_register_component"},
		{&ibus.factoryNew, "ibus_factory_new"},
		{&ibus.componentNew, "ibus_component_new"},
		{&ibus.engineDescNew, "ibus_engine_desc_new"},
		{&ibus.componentAdd, "ibus_component_add_engine"},
		{&ibus.engineType, "ibus_engine_get_type"},
		{&ibus.destroy, "ibus_object_destroy"},
		{&ibus.textNew, "ibus_text_new_from_string"},
		{&ibus.commit, "ibus_engine_commit_text"},
		{&ibus.preedit, "ibus_engine_update_preedit_text_with_mode"},
		{&ibus.keyvalToUnicode, "ibus_keyval_to_unicode"},
	} {
		bind.req(entry.fn, entry.name, lib)
	}
	bind.req(&ibus.objectNew, "g_object_new", libGObject)
	if len(bind.missing) != 0 {
		ibusErr = fmt.Errorf("missing IBus functions: %s", strings.Join(bind.missing, ", "))
		return
	}
	ibus.init()
	ibus.create = purego.NewCallback(func(_ ptr, name ptr, _ ptr) ptr {
		return ibusHost.newEngine(goStr(name))
	})
	ibus.in = purego.NewCallback(func(engine, path, client, _ ptr) {
		ibusHost.focusIn(engine, goStr(path), goStr(client))
	})
	ibus.out = purego.NewCallback(func(engine, path, _ ptr) {
		ibusHost.focusOut(engine, goStr(path))
	})
	ibus.key = purego.NewCallback(func(engine ptr, keyval, hardware, state uint32, _ ptr) ptr {
		const release = 1 << 30
		if state&release != 0 {
			return 0
		}
		session := ibusHost.session(engine)
		key := ibusKey(keyval, hardware, state, rune(ibus.keyvalToUnicode(keyval)))
		return gbool(ibusHost.sink.IMEKey(session.id, key))
	})
	ibus.caret = purego.NewCallback(func(engine ptr, x, y, width, height int32, _ ptr) {
		ibusHost.session(engine).caret = platform.Rect{X: int(x), Y: int(y), Width: int(width), Height: int(height)}
	})
})

type ibusSession struct {
	id      uint64
	engine  ptr
	path    string
	client  string
	caret   platform.Rect
	leaving bool
}

type ibusServer struct {
	bus, factory, connection ptr
	name                     string
	sink                     platform.InputMethodSink
	nextEngine               uint64
	nextSession              uint64
	byEngine                 map[ptr]*ibusSession
	byID                     map[uint64]*ibusSession
}

var _ platform.InputMethods = (*backend)(nil)

func (b *backend) ServeInputMethod(connection string, sink platform.InputMethodSink) error {
	if ibusHost != nil {
		return errors.New("tango/linux: input method already serving")
	}
	loadIBus()
	if ibusErr != nil {
		return fmt.Errorf("tango/linux: input method: %w", ibusErr)
	}
	if connection == "" {
		connection = b.opts.ID + "_Connection"
	}
	name := strings.TrimSuffix(connection, "_Connection")
	if name == "" || strings.ContainsAny(name, "\x00 \t\n") {
		return errors.New("tango/linux: invalid IBus engine name")
	}
	bus := ibus.busNew()
	if bus == 0 {
		return errors.New("tango/linux: cannot create IBus connection")
	}
	if !ibus.busConnected(bus) {
		gObjectUnref(bus)
		return errors.New("tango/linux: IBus daemon is not running")
	}
	server := &ibusServer{bus: bus, connection: ibus.busConnection(bus), name: name, sink: sink,
		byEngine: make(map[ptr]*ibusSession), byID: make(map[uint64]*ibusSession)}
	server.factory = ibus.factoryNew(server.connection)
	if server.factory == 0 {
		gObjectUnref(bus)
		return errors.New("tango/linux: cannot create IBus engine factory")
	}
	ibusHost = server
	gSignalConnectData(server.factory, "create-engine", ibus.create, 0, 0, 0)
	displayName := b.opts.Name
	if displayName == "" {
		displayName = name
	}
	hash := sha256.Sum256([]byte(name))
	componentName := fmt.Sprintf("org.freedesktop.IBus.Tango.Engine%x", hash[:8])
	component := ibus.componentNew(componentName, displayName, "1", "", "", "", "", "")
	desc := ibus.engineDescNew(name, displayName, displayName, "", "", "", "", "default")
	ibus.componentAdd(component, desc)
	registered := ibus.busRegister(bus, component)
	gObjectUnref(component)
	if !registered {
		server.close()
		return fmt.Errorf("tango/linux: cannot register IBus engine %q", name)
	}
	b.inputMethod = server
	return nil
}

func (s *ibusServer) newEngine(name string) ptr {
	if name != s.name {
		return 0
	}
	s.nextEngine++
	path := "/org/freedesktop/IBus/Engine/Tango/" + strconv.FormatUint(s.nextEngine, 10)
	engine := ibus.objectNew(ibus.engineType(), "engine-name", name, "object-path", path,
		"connection", s.connection, "has-focus-id", int32(1), uintptr(0))
	if engine == 0 {
		return 0
	}
	gSignalConnectData(engine, "focus-in-id", ibus.in, 0, 0, 0)
	gSignalConnectData(engine, "focus-out-id", ibus.out, 0, 0, 0)
	gSignalConnectData(engine, "process-key-event", ibus.key, 0, 0, 0)
	gSignalConnectData(engine, "set-cursor-location", ibus.caret, 0, 0, 0)
	connect(engine, "focus-in", 0, func(ptr, ptr) ptr { s.focusIn(engine, "", ""); return 0 })
	connect(engine, "focus-out", 0, func(ptr, ptr) ptr { s.focusOut(engine, ""); return 0 })
	connect(engine, "disable", 0, func(ptr, ptr) ptr { s.focusOut(engine, ""); return 0 })
	connect(engine, "reset", 0, func(ptr, ptr) ptr {
		if session := s.byEngine[engine]; session != nil {
			s.sink.IMECommitComposition(session.id)
		}
		return 0
	})
	connect(engine, "destroy", 0, func(ptr, ptr) ptr { s.focusOut(engine, ""); return 0 })
	return engine
}

// session returns the engine's session, activating a new one. A key or
// cursor update can arrive without focus-in, such as after disable and
// enable in the same text field.
func (s *ibusServer) session(engine ptr) *ibusSession {
	if session := s.byEngine[engine]; session != nil {
		return session
	}
	s.nextSession++
	session := &ibusSession{id: s.nextSession, engine: engine}
	s.byEngine[engine], s.byID[session.id] = session, session
	s.sink.IMEActivate(session.id)
	return session
}

func (s *ibusServer) focusIn(engine ptr, path, client string) {
	if active := s.byEngine[engine]; active != nil && active.path != "" {
		if path == "" || active.path == path {
			return
		}
		s.focusOut(engine, active.path)
	}
	session := s.session(engine)
	session.path, session.client = path, client
}

func (s *ibusServer) focusOut(engine ptr, path string) {
	if session := s.byEngine[engine]; session != nil && (path == "" || session.path == path) {
		// IBus commits the last preedit before notifying the engine of focus loss.
		session.leaving = true
		s.sink.IMEDeactivate(session.id)
		delete(s.byEngine, engine)
		delete(s.byID, session.id)
		s.sink.IMEEnd(session.id)
	}
}

func (s *ibusServer) close() {
	ibus.destroy(s.factory)
	gObjectUnref(s.factory)
	gObjectUnref(s.bus)
	ibusHost = nil
}

func (b *backend) IMESetComposition(id uint64, text string, cursor int) {
	if session := b.inputMethod.byID[id]; session != nil && !session.leaving {
		cursor = min(max(cursor, 0), utf8.RuneCountInString(text))
		const preeditCommitOnBlur = 1 // IBUS_ENGINE_PREEDIT_COMMIT
		ibus.preedit(session.engine, ibus.textNew(text), uint32(cursor), int32(gbool(text != "")), preeditCommitOnBlur)
	}
}

func (b *backend) IMECommit(id uint64, text string) {
	if session := b.inputMethod.byID[id]; session != nil && !session.leaving {
		if text != "" {
			ibus.commit(session.engine, ibus.textNew(text))
		}
		ibus.preedit(session.engine, ibus.textNew(""), 0, 0, 0)
	}
}

func (b *backend) IMECaret(id uint64) platform.Rect {
	if session := b.inputMethod.byID[id]; session != nil {
		return session.caret
	}
	return platform.Rect{}
}

func (b *backend) IMEClientApp(id uint64) string {
	if session := b.inputMethod.byID[id]; session != nil {
		return session.client
	}
	return ""
}
