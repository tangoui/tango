"""Expose the text from a focused GTK entry for the Xvfb keyboard smoke test."""
import os
from pathlib import Path

import gi

gi.require_version("Gtk", "3.0")
from gi.repository import Gtk

text_path = Path(os.environ["TANGO_SMOKE_TEXT"])
focus_path = text_path.with_suffix(".focus")
preedit_path = text_path.with_suffix(".preedit")
window = Gtk.Window(title="Tango IME Smoke")
window.set_default_size(400, 100)
entries = Gtk.Box(orientation=Gtk.Orientation.VERTICAL, homogeneous=True)
entry = Gtk.Entry()
other = Gtk.Entry()
entries.pack_start(entry, True, True, 0)
entries.pack_start(other, True, True, 0)
window.add(entries)
entry.connect("changed", lambda widget: text_path.write_text(widget.get_text(), encoding="utf-8"))
entry.connect("preedit-changed", lambda _widget, text: preedit_path.write_text(text, encoding="utf-8"))
def record_focus(_widget, _event, name):
    focus_path.write_text(name, encoding="utf-8")
    return False


entry.connect("focus-in-event", record_focus, "primary")
other.connect("focus-in-event", record_focus, "secondary")
window.connect("destroy", Gtk.main_quit)
window.show_all()
entry.grab_focus()
Gtk.main()
