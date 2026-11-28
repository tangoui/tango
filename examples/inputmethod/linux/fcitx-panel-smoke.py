"""Check focus-out from a Fcitx input context with only panel preedit."""
import os
import subprocess
import time

from gi.repository import Gio, GLib

bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
service = "org.fcitx.Fcitx5"
method_interface = "org.fcitx.Fcitx.InputMethod1"
context_interface = "org.fcitx.Fcitx.InputContext1"


def call(path, interface, method, signature=None, args=()):
    data = GLib.Variant(signature, args) if signature else None
    return bus.call_sync(service, path, interface, method, data, None,
                         Gio.DBusCallFlags.NONE, 2000, None).unpack()


path, _ = call("/org/freedesktop/portal/inputmethod", method_interface, "CreateInputContext", "(a(ss))",
               ([("program", "tango-panel-smoke"), ("display", "x11:" + os.environ["DISPLAY"])],))
commits = []
subscription = bus.signal_subscribe(
    service, context_interface, "CommitString", path, None, Gio.DBusSignalFlags.NONE,
    lambda _conn, _sender, _path, _iface, _signal, parameters: commits.append(parameters.unpack()[0]),
)


def context_call(method, signature=None, args=()):
    return call(path, context_interface, method, signature, args)


try:
    context_call("SetCapability", "(t)", (0,))
    context_call("FocusIn")
    subprocess.run(["fcitx5-remote", "-o"], check=True)
    subprocess.run(["fcitx5-remote", "-s", "com.tangoui.inputmethod.example"], check=True)
    # The addon passes keys through until the host finishes its handshake.
    deadline = time.monotonic() + 5
    while not context_call("ProcessKeyEvent", "(uuubu)", (ord("n"), 0, 0, False, 0))[0]:
        assert time.monotonic() < deadline, "Tango did not handle the first key"
        time.sleep(0.1)
    assert context_call("ProcessKeyEvent", "(uuubu)", (ord("i"), 0, 0, False, 0))[0]
    print("Fcitx5 handled both keys in a context without client preedit", flush=True)
    context_call("FocusOut")
    deadline = time.monotonic() + 1
    loop = GLib.MainContext.default()
    while time.monotonic() < deadline:
        while loop.pending():
            loop.iteration(False)
        time.sleep(0.005)
    assert commits == ["ni"], commits
    print("Fcitx5 commits panel-only preedit once on focus-out")
finally:
    context_call("DestroyIC")
    bus.signal_unsubscribe(subscription)
