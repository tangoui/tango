package tango

import "github.com/tangoui/tango/internal/platform"

// App events of phones and tablets (docs/specs/mobile-canvas.md §3.4).

// EventMemoryWarning: the system is low on memory (an iOS memory warning,
// Android onTrimMemory at TRIM_MEMORY_RUNNING_LOW or above). The event has
// no payload. Release what can be rebuilt (caches, decoded images, data
// that can be reloaded); windows opened with uihost already drop their GPU
// atlases, image textures and shaped-text caches and redraw. Desktop
// platforms do not send it. Register with
// App.OnEventKind(EventMemoryWarning, ...).
const EventMemoryWarning = platform.AppMemoryWarning
