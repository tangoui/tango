package ui

// ConfigProvider.Defaults wiring of the navigation and feedback components
// (Ant Design prop names; explicit modifiers always win because they run
// after the constructor applied these):
//
//	Tabs        type, size, centered, placement / tabPosition
//	Pagination  size, simple, showQuickJumper, showTotal, hideOnSinglePage, background, disabled
//	Breadcrumb  separator
//	Menu        mode, theme, accordion, inlineCollapsed
//	Dropdown    trigger, placement, arrow, disabled
//	Steps       type, size, labelPlacement, direction, progressDot
//	Modal       width, centered, closable, mask, maskClosable, okText, cancelText
//	Drawer      placement, size / width, closable, mask, maskClosable
//	Popconfirm  okText, cancelText, placement, showCancel
//	Alert       showIcon, closable, banner
//	Spin        size, delay, tip
//	Message     duration
//	Notification placement, duration, showProgress, pauseOnHover
//	Confirm     width, okText, cancelText (also Modal's okText / cancelText)
//	Toggle      size, variant ("outline")
//	ToggleGroup size, variant ("outline")
//	Splitter    layout ("vertical")
//	FloatButton shape ("square"), type ("primary")
//	Affix       offsetTop
//	Anchor      offsetTop / targetOffset, bounds
//	PageHeader  ghost
//	Space       size

import (
	"strings"
	"time"
)

// s2cSize reads a "size" default: a ControlSize or "small" / "middle" /
// "default" / "large".
func s2cSize(c *Context, comp string) (ControlSize, bool) {
	v, ok := c.ConfigProp(comp, "size")
	if !ok {
		return Middle, false
	}
	switch x := v.(type) {
	case ControlSize:
		return x, true
	case string:
		switch strings.ToLower(x) {
		case "small", "sm":
			return Small, true
		case "large", "lg":
			return Large, true
		case "middle", "default", "medium":
			return Middle, true
		}
	}
	return Middle, false
}

// s2cPlacement parses Ant's placement names ("bottomLeft", "top", …) or a
// Placement value.
func s2cPlacement(v any) (Placement, bool) {
	switch x := v.(type) {
	case Placement:
		return x, true
	case string:
		p, ok := map[string]Placement{
			"bottomleft": BottomStart, "bottom": Bottom, "bottomright": BottomEnd,
			"topleft": TopStart, "top": Top, "topright": TopEnd,
			"bottom-start": BottomStart, "bottom-end": BottomEnd, "top-start": TopStart, "top-end": TopEnd,
		}[strings.ToLower(x)]
		return p, ok
	}
	return 0, false
}

func (d *DropdownW) defaults() {
	c := d.c
	if v, ok := p2bStr(c, "Dropdown", "trigger"); ok {
		switch strings.ToLower(v) {
		case "click":
			d.trigger = TriggerClick
		case "contextmenu":
			d.trigger = TriggerContextMenu
		case "hover":
			d.trigger = TriggerHover
		}
	} else if t, ok := c.ConfigProp("Dropdown", "trigger"); ok {
		if tr, ok := t.(Trigger); ok {
			d.trigger = tr
		}
	}
	if v, ok := c.ConfigProp("Dropdown", "placement"); ok {
		if p, ok := s2cPlacement(v); ok {
			d.place, d.placeSet = p, true
		}
	}
	if v, ok := p2bBool(c, "Dropdown", "arrow"); ok {
		d.arrow = v
	}
	if v, ok := p2bBool(c, "Dropdown", "disabled"); ok {
		d.disabled = v
	}
}

func (m *ModalW) defaults() {
	c := m.c
	if v, ok := p2bNum(c, "Modal", "width"); ok {
		m.width = float32(v)
	}
	if v, ok := p2bBool(c, "Modal", "centered"); ok {
		m.centered = v
	}
	if v, ok := p2bBool(c, "Modal", "closable"); ok {
		m.closable = v
	}
	if v, ok := p2bBool(c, "Modal", "mask"); ok {
		m.mask = v
	}
	if v, ok := p2bBool(c, "Modal", "maskClosable"); ok {
		m.maskClose = v
	}
	if v, ok := p2bStr(c, "Modal", "okText"); ok {
		m.okText = v
	}
	if v, ok := p2bStr(c, "Modal", "cancelText"); ok {
		m.cancelText = v
	}
}

func (d *DrawerW) defaults() {
	c := d.c
	if v, ok := c.ConfigProp("Drawer", "placement"); ok {
		switch x := v.(type) {
		case DrawerPlacement:
			d.place = x
		case string:
			// Ant placement names are logical (spec Q2): "left" is the
			// start edge, "right" the end edge (default).
			d.place = map[string]DrawerPlacement{"left": DrawerStart, "start": DrawerStart, "top": DrawerTop, "bottom": DrawerBottom}[strings.ToLower(x)]
		}
	}
	if v, ok := c.ConfigProp("Drawer", "size"); ok {
		if s, ok := v.(string); ok && s == "large" {
			d.size = 736
		} else if n, ok := p2bNum(c, "Drawer", "size"); ok {
			d.size = float32(n)
		}
	} else if n, ok := p2bNum(c, "Drawer", "width"); ok {
		d.size = float32(n)
	}
	if v, ok := p2bBool(c, "Drawer", "closable"); ok {
		d.closable = v
	}
	if v, ok := p2bBool(c, "Drawer", "mask"); ok {
		d.mask = v
	}
	if v, ok := p2bBool(c, "Drawer", "maskClosable"); ok {
		d.maskClose = v
	}
}

func (s *SpinW) defaults() {
	c := s.c
	if sz, ok := s2cSize(c, "Spin"); ok {
		s.size, s.sizeSet = sz, true
	}
	if d, ok := p2bDur(c, "Spin", "delay"); ok {
		s.delay = d
	}
	if t, ok := p2bStr(c, "Spin", "tip"); ok {
		s.tip = t
	}
}

func (s *StepsW) defaults() {
	c := s.c
	if sz, ok := s2cSize(c, "Steps"); ok {
		s.size, s.sizeSet = sz, true
	}
	if v, ok := p2bStr(c, "Steps", "direction"); ok {
		s.vertical = v == "vertical"
	}
	if v, ok := p2bBool(c, "Steps", "progressDot"); ok && v {
		s.Dot()
	}
}

// messageDuration returns the Message duration default (seconds as a
// number, Ant style, or a time.Duration).
func messageDuration(c *Context, comp string, def time.Duration) time.Duration {
	v, ok := c.ConfigProp(comp, "duration")
	if !ok {
		return def
	}
	if d, ok := v.(time.Duration); ok {
		return d
	}
	if n, ok := p2bNum(c, comp, "duration"); ok {
		if n <= 0 {
			return -1 // stays until closed
		}
		return time.Duration(n * float64(time.Second))
	}
	return def
}

func (n *navNote) defaults(c *Context) {
	if v, ok := c.ConfigProp("Notification", "placement"); ok {
		switch x := v.(type) {
		case NotifyPlacement:
			n.cfg.Placement = x
		case string:
			// Ant placement names are logical (spec Q2): left is the
			// start side, right the end side (topRight is the default).
			n.cfg.Placement = map[string]NotifyPlacement{"topleft": NotifyTopStart, "bottomright": NotifyBottomEnd,
				"bottomleft": NotifyBottomStart, "top": NotifyTop, "bottom": NotifyBottom}[strings.ToLower(x)]
		}
	}
	if _, ok := c.ConfigProp("Notification", "duration"); ok {
		n.cfg.Duration = messageDuration(c, "Notification", 0)
	}
	if v, ok := p2bBool(c, "Notification", "showProgress"); ok {
		n.cfg.ShowProgress = v
	}
	if v, ok := p2bBool(c, "Notification", "pauseOnHover"); ok {
		n.cfg.NoPause = !v
	}
}

func (o *ConfirmOptions) defaults(c *Context) {
	if o.Width == 0 {
		if v, ok := p2bNum(c, "Confirm", "width"); ok {
			o.Width = float32(v)
		}
	}
	for _, comp := range []string{"Confirm", "Modal"} {
		if o.OkText == "" {
			o.OkText, _ = p2bStr(c, comp, "okText")
		}
		if o.CancelText == "" {
			o.CancelText, _ = p2bStr(c, comp, "cancelText")
		}
	}
}
