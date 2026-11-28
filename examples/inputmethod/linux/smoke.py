import gi
import time
gi.require_version('IBus', '1.0')
from gi.repository import IBus, GLib
IBus.init()
bus = IBus.Bus()
assert bus.is_connected()
panel = IBus.PanelService.new(bus.get_connection())
assert bus.request_name("org.freedesktop.IBus.Panel", 0) == 1
ctx = bus.create_input_context('tango-integration')
ctx.set_capabilities(IBus.Capabilite.PREEDIT_TEXT | IBus.Capabilite.FOCUS)
ctx.focus_in()
seen = []
ctx.connect('update-preedit-text', lambda _, text, cursor, visible: seen.append(('preedit', text.get_text(), cursor, visible)))
ctx.connect('commit-text', lambda _, text: seen.append(('commit', text.get_text())))
def drain(ms):
    until = time.monotonic() + ms / 1000
    context = GLib.MainContext.default()
    while time.monotonic() < until:
        while context.pending():
            context.iteration(False)
        time.sleep(0.003)
for _ in range(40):
    ctx.set_engine('com.tangoui.inputmethod.example')
    drain(100)
    engine = ctx.get_engine()
    if engine is not None and engine.get_name() == 'com.tangoui.inputmethod.example':
        break
else:
    raise RuntimeError('Tango engine did not register')
for ch in 'nihao':
    assert ctx.process_key_event(ord(ch), 0, 0), ch
drain(150)
assert ('preedit', 'nihao', 5, True) in seen, seen
assert ctx.process_key_event(32, 57, 0)
for ch in 'shijie':
    assert ctx.process_key_event(ord(ch), 0, 0), ch
assert ctx.process_key_event(IBus.KEY_Escape, 1, 0)
for ch in 'wo':
    assert ctx.process_key_event(ord(ch), 0, 0), ch
assert ctx.process_key_event(IBus.KEY_Return, 28, 0)
drain(200)
assert [item[1] for item in seen if item[0] == 'commit'] == ['你好', 'wo'], seen
print(seen)
ctx.focus_out()
ctx.focus_in()
drain(100)
assert ctx.process_key_event(ord('n'), 0, 0)
assert ctx.process_key_event(ord('i'), 0, 0)
drain(100)
ctx.focus_out()
drain(200)
print('after blur:', seen[-10:])
assert [item[1] for item in seen if item[0] == 'commit'] == ['你好', 'wo', 'ni'], seen
ctx.focus_in()
drain(100)
for ch in 'ni':
    assert ctx.process_key_event(ord(ch), 0, 0), ch
panel.candidate_clicked(1, 1, 0)
drain(200)
assert [item[1] for item in seen if item[0] == 'commit'] == ['你好', 'wo', 'ni', '泥'], seen
print("IBus panel selection commits the second candidate")
ctx.focus_out()
