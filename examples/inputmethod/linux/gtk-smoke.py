"""Expose the text from a focused GTK entry for the Xvfb keyboard smoke test."""
import os
from pathlib import Path

import gi

gi.require_version("Gtk", "3.0")
from gi.repository import Gtk

text_path = Path(os.environ["TANGO_SMOKE_TEXT"])
window = Gtk.Window(title="Tango IME Smoke")
window.set_default_size(400, 100)
entry = Gtk.Entry()
window.add(entry)
entry.connect("changed", lambda widget: text_path.write_text(widget.get_text(), encoding="utf-8"))
window.connect("destroy", Gtk.main_quit)
window.show_all()
entry.grab_focus()
Gtk.main()
