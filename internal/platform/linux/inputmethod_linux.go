//go:build linux && !android

package linux

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/ebitengine/purego"
	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/imesession"
)

var ibus struct {
	init                                  func()
	busNew                                func() ptr
	busConnected                          func(ptr) bool
	busConnection                         func(ptr) ptr
	busRegister                           func(ptr, ptr) bool
	busRequestName                        func(ptr, string, uint32) uint32
	lookupNew                             func(uint32, uint32, int32, int32) ptr
	lookupAppend                          func(ptr, ptr)
	updateLookup                          func(ptr, ptr, int32)
	propNew                               func(string, uint32, ptr, string, ptr, int32, int32, uint32, ptr) ptr
	propListNew                           func() ptr
	propListAppend                        func(ptr, ptr)
	registerProps                         func(ptr, ptr)
	factoryNew                            func(ptr) ptr
	componentNew                          func(string, string, string, string, string, string, string, string) ptr
	engineDescNew                         func(string, string, string, string, string, string, string, string) ptr
	componentAdd                          func(ptr, ptr)
	engineType                            func() ptr
	objectNew                             func(ptr, string, ...any) ptr
	destroy                               func(ptr)
	textNew                               func(string) ptr
	commit                                func(ptr, ptr)
	preedit                               func(ptr, ptr, uint32, int32, uint32)
	keyvalToUnicode                       func(uint32) uint32
	create, key, in, out, caret, property ptr
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
		{&ibus.busRequestName, "ibus_bus_request_name"},
		{&ibus.lookupNew, "ibus_lookup_table_new"},
		{&ibus.lookupAppend, "ibus_lookup_table_append_candidate"},
		{&ibus.updateLookup, "ibus_engine_update_lookup_table"},
		{&ibus.propNew, "ibus_property_new"},
		{&ibus.propListNew, "ibus_prop_list_new"},
		{&ibus.propListAppend, "ibus_prop_list_append"},
		{&ibus.registerProps, "ibus_engine_register_properties"},
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
		session := ibusHost.session(engine)
		repeat := session.held.event(keyval, hardware, state)
		if mod, ok := ibusModifier(keyval, state); ok {
			return gbool(!repeat && ibusHost.mux.Modifier(session.routed, mod))
		}
		if state&ibusReleaseMask != 0 {
			return 0
		}
		key := ibusKey(keyval, hardware, state, rune(ibus.keyvalToUnicode(keyval)))
		key.Repeat = repeat
		return gbool(ibusHost.mux.Key(session.routed, key))
	})
	ibus.property = purego.NewCallback(func(engine, a ptr, state uint32, _ ptr) {
		s := ibusHost
		if session := s.byEngine[engine]; session != nil {
			key := goStr(a)
			switch {
			case strings.HasPrefix(key, "mode:") && state == 1:
				_ = s.mux.SetMode(strings.TrimPrefix(key, "mode:"), true)
			case strings.HasPrefix(key, "menu:"):
				s.mux.Dispatch(session.transport, proto.Message{Body: proto.MenuPick(strings.TrimPrefix(key, "menu:"))})
			}
		}
	})

	ibus.caret = purego.NewCallback(func(engine ptr, x, y, width, height int32, _ ptr) {
		ibusHost.session(engine).routed.SetCaret(platform.Rect{X: int(x), Y: int(y), Width: int(width), Height: int(height)})
	})
})

type ibusSession struct {
	id        uint64
	engine    ptr
	path      string
	routed    *imesession.Session
	transport *imesession.Client
	menu      []proto.MenuItem
	held      ibusHeldKeys
	leaving   bool
}

type ibusServer struct {
	bus, factory, connection ptr
	name                     string
	mux                      *imesession.Mux
	modes                    []proto.Mode
	nextEngine               uint64
	byEngine                 map[ptr]*ibusSession
	byID                     map[uint64]*ibusSession
}

var _ platform.InputMethods = (*backend)(nil)

type linuxIMEServer struct {
	ibus  *ibusServer
	fcitx *fcitxServer
	mux   *imesession.Mux
}

func (s *linuxIMEServer) close() {
	if s.fcitx != nil {
		s.fcitx.close()
	}
	if s.ibus != nil {
		s.ibus.close()
	}
}

func (s *linuxIMEServer) ibusSession(id uint64) *ibusSession {
	if s.ibus == nil {
		return nil
	}
	return s.ibus.byID[id]
}

func (b *backend) ServeInputMethod(connection string, sink platform.InputMethodSink) error {
	if b.inputMethod != nil {
		return errors.New("tango/linux: input method already serving")
	}
	cfg, err := readLinuxIMEConfig(b.opts.ID)
	if err != nil {
		return err
	}
	if connection == "" {
		connection = cfg.Connection
	}
	if connection == "" {
		connection = b.opts.DefaultIMEConnection()
	}
	mux := imesession.New(sink, cfg.Modes, fcitxKey)
	fcitx, fcitxErr := startFcitx(connection, mux)

	// Preserve runtime registration for the IBus sessions which do not
	// export their address (libibus can derive it from the display).
	wantIBus := os.Getenv("IBUS_ADDRESS") != "" || slices.Contains(os.Args[1:], "--ibus") ||
		os.Getenv("GTK_IM_MODULE") == "ibus" || os.Getenv("XMODIFIERS") == "@im=ibus"
	fcitxDesktop := os.Getenv("GTK_IM_MODULE") == "fcitx" || os.Getenv("XMODIFIERS") == "@im=fcitx"
	var bus *ibusServer
	var ibusErr error
	if wantIBus || !fcitxDesktop {
		bus, ibusErr = b.startIBus(connection, cfg, mux)
	}
	if bus == nil && fcitx == nil {
		return fmt.Errorf("tango/linux: neither input method transport is available: %w",
			errors.Join(fcitxErr, ibusErr))
	}
	if slices.Contains(os.Args[1:], "--ibus") && bus == nil {
		fcitx.close()
		return ibusErr
	}
	b.inputMethod = &linuxIMEServer{ibus: bus, fcitx: fcitx, mux: mux}
	return nil
}

func (b *backend) startIBus(connection string, cfg linuxIMEConfig, mux *imesession.Mux) (*ibusServer, error) {
	if ibusHost != nil {
		return nil, errors.New("tango/linux: input method already serving")
	}
	loadIBus()
	if ibusErr != nil {
		return nil, fmt.Errorf("tango/linux: input method: %w", ibusErr)
	}
	name := cfg.IBusEngine
	if name == "" {
		name = strings.TrimSuffix(connection, "_Connection")
	}
	if name == "" || strings.ContainsAny(name, "\x00 \t\n") {
		return nil, errors.New("tango/linux: invalid IBus engine name")
	}
	bus := ibus.busNew()
	if bus == 0 {
		return nil, errors.New("tango/linux: cannot create IBus connection")
	}
	if !ibus.busConnected(bus) {
		gObjectUnref(bus)
		return nil, errors.New("tango/linux: IBus daemon is not running")
	}
	server := &ibusServer{bus: bus, connection: ibus.busConnection(bus), name: name, mux: mux, modes: cfg.Modes,
		byEngine: make(map[ptr]*ibusSession), byID: make(map[uint64]*ibusSession)}
	server.factory = ibus.factoryNew(server.connection)
	if server.factory == 0 {
		gObjectUnref(bus)
		return nil, errors.New("tango/linux: cannot create IBus engine factory")
	}
	ibusHost = server
	gSignalConnectData(server.factory, "create-engine", ibus.create, 0, 0, 0)
	displayName := b.opts.Name
	if displayName == "" {
		displayName = name
	}
	componentName := ibusComponentName(name)
	if cfg.IBusEngine != "" || slices.Contains(os.Args[1:], "--ibus") {
		const primaryOwner = 1
		if ibus.busRequestName(bus, componentName, 0) != primaryOwner {
			server.close()
			return nil, fmt.Errorf("tango/linux: cannot own IBus component %q", componentName)
		}
		return server, nil
	}
	component := ibus.componentNew(componentName, displayName, "1", "", "", "", "", "")
	desc := ibus.engineDescNew(name, displayName, displayName, "", "", "", "", "default")
	ibus.componentAdd(component, desc)
	registered := ibus.busRegister(bus, component)
	gObjectUnref(component)
	if !registered {
		server.close()
		return nil, fmt.Errorf("tango/linux: cannot register IBus engine %q", name)
	}
	return server, nil
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
			s.mux.CommitComposition(session.routed)
		}
		return 0
	})
	gSignalConnectData(engine, "property-activate", ibus.property, 0, 0, 0)
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
	session := &ibusSession{engine: engine, held: make(ibusHeldKeys)}
	session.transport = s.mux.Connect("", func(batch []proto.Message) {
		for _, frame := range batch {
			switch value := frame.Body.(type) {
			case proto.Menu:
				session.menu = value.Items
				s.publishProps(session)
			case proto.Mode:
				s.publishProps(session)
			}
		}
	})
	session.routed = s.mux.Open(session.transport, 1)
	session.id = session.routed.ID()
	s.byEngine[engine], s.byID[session.id] = session, session
	s.mux.Focus(session.routed, true)
	s.mux.SendMenu(session.transport)
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
	session.path = path
	session.routed.SetClientApp(client)
}

func (s *ibusServer) focusOut(engine ptr, path string) {
	if session := s.byEngine[engine]; session != nil && (path == "" || session.path == path) {
		// IBus commits the last preedit before notifying the engine of focus loss.
		session.leaving = true
		s.mux.Disconnect(session.transport)
		delete(s.byEngine, engine)
		delete(s.byID, session.id)
	}
}

func (s *ibusServer) close() {
	ibus.destroy(s.factory)
	gObjectUnref(s.factory)
	gObjectUnref(s.bus)
	ibusHost = nil
}

func (b *backend) IMESetComposition(id uint64, text string, cursor int) {
	if session := b.inputMethod.ibusSession(id); session != nil && !session.leaving {
		cursor = min(max(cursor, 0), utf8.RuneCountInString(text))
		const preeditCommitOnBlur = 1 // IBUS_ENGINE_PREEDIT_COMMIT
		ibus.preedit(session.engine, ibus.textNew(text), uint32(cursor), int32(gbool(text != "")), preeditCommitOnBlur)
	} else if session == nil {
		if routed := b.inputMethod.mux.Session(id); routed != nil {
			cursor = min(max(cursor, 0), utf8.RuneCountInString(text))
			routed.Post(proto.Compose{Session: routed.RemoteID(), Text: text, Cursor: uint32(cursor)})
		}
	}
}

func (b *backend) IMECommit(id uint64, text string) {
	if session := b.inputMethod.ibusSession(id); session != nil && !session.leaving {
		if text != "" {
			ibus.commit(session.engine, ibus.textNew(text))
		}
		ibus.preedit(session.engine, ibus.textNew(""), 0, 0, 0)
	} else if session == nil {
		if routed := b.inputMethod.mux.Session(id); routed != nil {
			routed.Post(proto.Commit{Session: routed.RemoteID(), Text: text})
		}
	}
}

func (b *backend) IMECaret(id uint64) platform.Rect {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		if session := b.inputMethod.ibusSession(id); session != nil {
			return session.routed.Caret()
		}
	}
	return platform.Rect{}
}

func (b *backend) IMEClientApp(id uint64) string {
	if session := b.inputMethod.mux.Session(id); session != nil {
		return session.ClientApp()
	}
	return ""
}

func (s *ibusServer) publishProps(session *ibusSession) {
	if s.factory == 0 || session.routed == nil || session.leaving {
		return
	}
	ibus.registerProps(session.engine, makeIBusProps(ibusProps(s.modes, s.mux.CurrentMode(), session.menu)))
}

func makeIBusProps(definitions []ibusProperty) ptr {
	list := ibus.propListNew()
	for _, property := range definitions {
		var children ptr
		if len(property.children) != 0 {
			children = makeIBusProps(property.children)
		}
		state := uint32(0)
		if property.checked {
			state = 1
		}
		p := ibus.propNew(property.key, property.kind, ibus.textNew(property.label), "",
			ibus.textNew(""), int32(gbool(!property.disabled)), 1, state, children)
		ibus.propListAppend(list, p)
	}
	return list
}

func (b *backend) IMESetCandidates(id uint64, list []platform.IMECandidate, selected int) {
	if session := b.inputMethod.ibusSession(id); session != nil && !session.leaving {
		ibusSetCandidates(session.engine, list, selected)
	} else if session == nil {
		if routed := b.inputMethod.mux.Session(id); routed != nil {
			items := make([]proto.Candidate, len(list))
			for i, item := range list {
				items[i] = proto.Candidate(item)
			}
			routed.Post(proto.Candidates{Session: routed.RemoteID(), Selected: int32(selected), Items: items})
		}
	}
}

func ibusSetCandidates(engine ptr, list []platform.IMECandidate, selected int) {
	table := newIBusLookup(list, selected)
	native := ibus.lookupNew(uint32(table.pageSize), uint32(table.cursor), int32(gbool(table.cursorVisible)), 0)
	for _, candidate := range table.candidates {
		ibus.lookupAppend(native, ibus.textNew(ibusCandidateLabel(candidate)))
	}
	ibus.updateLookup(engine, native, int32(gbool(len(list) != 0)))
}

func (b *backend) IMESetMode(id string) error {
	if b.inputMethod == nil {
		return platform.ErrUnsupported
	}
	return b.inputMethod.mux.SetMode(id, false)
}

func (b *backend) IMECurrentMode() string {
	if b.inputMethod == nil {
		return ""
	}
	return b.inputMethod.mux.CurrentMode()
}
