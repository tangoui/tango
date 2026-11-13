// Package mobile provides touch-first widgets for the Tango self-drawn UI
// in the visual language of Ant Design Mobile (default), Vant and a
// mobile derivation of shadcn/ui (Shadcn: chosen automatically under a
// theme.Shadcn ui theme; Drawer-style bottom sheets with a grab handle,
// grouped action sheet cards, Sonner toasts, segmented tabs): NavBar,
// TabBar, List/Cell, SwipeCell, PullToRefresh, InfiniteList, ActionSheet,
// Popup, Dialog, Toast, Picker (cascade, date/time), Stepper, SearchBar,
// Tabs, Swiper, Grid, NoticeBar, Skeleton, Empty, Result, Badge, Tag,
// Button, Form, IndexBar, FloatingBubble and SafeArea.
//
// Gestures (tap, long press, swipe with direction and speed, drag,
// inertial scrolling, rubber-band edges) are recognized from the ui
// layer's pointer events by a gesture arena (see Area and Touchable);
// pinch comes from the ui layer's multi-touch events (input.TouchEvent).
// Drags that start on plain ui widgets inside a scroll view or swipe cell
// are handed to the container by the ui layer's pointer takeover.
// The scroll physics (Scroller) and recognizers (Track, PinchRecognizer,
// RefreshMachine) are pure types usable on their own. Long lists of
// fixed-height rows build only the visible rows with ScrollViewW.Rows.
//
// Under a high-contrast ui theme (theme.HighContrast) the mobile neutrals
// are lifted to WCAG AA like the ui theme's (NewTheme).
//
// Components read ConfigProvider defaults under "mobile.<Component>"
// names (ui.ConfigProvider(c).Defaults("mobile.Button", ui.Props{"size":
// "large"})); explicit modifiers win. Each constructor documents its keys.
//
// On iOS and Android the widgets run in a self-drawn canvas page that
// uihost.Open shows natively (touch, safe area, keyboard, input methods
// and accessibility come from the platform; see docs/specs/mobile-canvas.md
// and examples/mobile-ui). On the desktop they run in any uihost window.
//
// Wrap a mobile page in Provider to pick the design style and safe area:
//
//	mobile.Provider(c, mobile.Options{Style: mobile.Vant}, func() {
//		mobile.NavBar(c, "标题").Back("")
//		mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() { ... })
//		mobile.TabBar(c, tabs, &active)
//	})
package mobile
