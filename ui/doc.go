// Package ui is the self-drawn GUI layer of Tango: an immediate-mode API
// over retained widget state, rendered by a pure-Go pipeline (no cgo) with
// the visual language of Ant Design v5 (default), Element Plus and shadcn/ui.
//
// # Programming model
//
// A page is a plain function called every frame:
//
//	func page(c *ui.Context) {
//		ui.Input(c, &name).Placeholder("用户名")
//		if ui.Button(c, "保存").Primary().Clicked() {
//			save(name)
//		}
//	}
//
// Calls build a tree of layout nodes; chained modifiers (Primary, Disabled,
// ...) configure the widget before layout and paint run at the end of the
// frame. Interaction (Clicked, Hovered, focus) is answered from the
// previous frame's geometry, keyed by widget ID. IDs derive from the
// enclosing scope, widget kind, label and occurrence; wrap dynamic lists in
// ui.Key. Values live in the app (pointers); widgets only retain transient
// state (caret, scroll, open dropdowns, animations) via ui.State.
// Frames are event driven: a host calls Runner.Frame after input or when the
// UI asks (Window.Invalidate / InvalidateAt for animations and timers); a
// static UI produces no frames.
//
// # Layers
//
//	ui/theme   design tokens (AntDesign, ElementPlus, Shadcn, Dark, derived palettes)
//	ui/layout  flex/stack/absolute solver with two-axis measurement
//	ui/paint   display list + Renderer interface + CPU rasterizer
//	ui/text    shaping (go-text/typesetting), fallback fonts, line breaking
//	ui/input   events, keys, IME composition, cursors, focus order
//	ui/icon    built-in vector icons
//	ui         Context, Runner, Driver, Window/Host contracts, components
//
// # Platform integration
//
// A platform backend implements Window (size, scale, invalidation, cursor,
// clipboard, IME, Present) and Host (open a window bound to a Runner); see
// window.go for the exact contract. The headless Driver implements Window
// for tests and screenshots. See AUTHORING.md for writing components.
//
// # Accessibility
//
// A window that also implements AccessibilityHost receives a semantics
// tree (package ui/a11y: roles, names, states, values, bounds, actions)
// after every frame that changed it, and forwards screen-reader requests
// through Runner.PerformAction. Nothing is built while the host reports
// accessibility disabled. Keyboard access (Tab order, roving arrow keys,
// focus traps in dialogs, focus return from popups), the reduced-motion
// preference (RunnerOptions.ReduceMotion) and theme contrast checks
// (theme.CheckContrast) complete the picture; docs/accessibility.md maps
// every role to NSAccessibility, UI Automation, AT-SPI, UIAccessibility
// and AccessibilityNodeInfo.
package ui
