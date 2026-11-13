package ui

import (
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// ConfigProvider

// Props are default properties of one component, by property name
// ("size", "round", "duration" ...). Components document the keys they read.
type Props map[string]any

type navConfig struct {
	locale   *Locale
	size     ControlSize
	hasSize  bool
	props    map[string]Props
	disabled bool
	dir      Direction // explicit direction of the nearest provider that set one (DirAuto: none)
}

type navConfigStack struct {
	frames []navConfig
}

var navConfigKey = hashID("nav-config-stack")

func (c *Context) navConfigs() *navConfigStack {
	return State[navConfigStack](c, navConfigKey, nil)
}

func (c *Context) navConfig() *navConfig {
	s := c.navConfigs()
	if len(s.frames) == 0 {
		return nil
	}
	return &s.frames[len(s.frames)-1]
}

// Locale returns the active locale (innermost ConfigProvider, else the
// default set by SetDefaultLocale, zh-CN out of the box). In paint and
// measure callbacks it is the locale of the scope the node was built in.
func (c *Context) Locale() *Locale {
	// The provider stack first: deferred work (lazy, choice.go) re-pushes
	// the frame of the scope it was created in. Paint and measure run with
	// an empty stack and read the node's env instead.
	if cf := c.navConfig(); cf != nil && cf.locale != nil {
		return cf.locale
	}
	if e := c.env; e != nil && e.loc != nil {
		return e.loc
	}
	localeMu.RLock()
	defer localeMu.RUnlock()
	return defaultLocale
}

// ComponentSize returns the size set by the innermost ConfigProvider and
// whether one was set; components use it when the caller gave no size.
func (c *Context) ComponentSize() (ControlSize, bool) {
	if cf := c.navConfig(); cf != nil && cf.hasSize {
		return cf.size, true
	}
	return Middle, false
}

// sizeOr returns the configured size when set is false.
func (c *Context) navSize(explicit ControlSize, set bool) ControlSize {
	if set {
		return explicit
	}
	s, _ := c.ComponentSize()
	return s
}

// ConfigProp returns a default property for component (nil, false when
// unset). Nested providers inherit and override per key.
func (c *Context) ConfigProp(component, key string) (any, bool) {
	cf := c.navConfig()
	if cf == nil || cf.props == nil {
		return nil, false
	}
	p, ok := cf.props[component]
	if !ok {
		return nil, false
	}
	v, ok := p[key]
	return v, ok
}

// Prop returns the typed default property, or def when it is unset or of
// another type.
func Prop[T any](c *Context, component, key string, def T) T {
	if v, ok := c.ConfigProp(component, key); ok {
		if t, ok := v.(T); ok {
			return t
		}
	}
	return def
}

// ConfigProviderW configures every component built inside its Body:
// size, locale, component default properties, disabled state and theme.
type ConfigProviderW struct {
	c       *Context
	cfg     navConfig
	th      *theme.Theme
	flavor  *theme.Flavor
	dark    *bool
	primary *theme.Color
	compact bool
	hc      *bool // HighContrast: nil inherits

	comps    theme.Components
	hasComps bool
}

// ConfigProvider starts a configuration scope; chain options, then Body.
func ConfigProvider(c *Context) *ConfigProviderW {
	p := &ConfigProviderW{c: c}
	if cf := c.navConfig(); cf != nil {
		p.cfg = *cf
		p.cfg.props = nil
		if cf.props != nil {
			p.cfg.props = make(map[string]Props, len(cf.props))
			for k, v := range cf.props {
				p.cfg.props[k] = v
			}
		}
	}
	return p
}

// Size sets the default control size (small / middle / large).
func (p *ConfigProviderW) Size(s ControlSize) *ConfigProviderW {
	p.cfg.size, p.cfg.hasSize = s, true
	return p
}

// Locale sets the language.
func (p *ConfigProviderW) Locale(l *Locale) *ConfigProviderW {
	if l != nil {
		p.cfg.locale = l
	}
	return p
}

// LocaleCode sets the language by registered code ("en-US").
func (p *ConfigProviderW) LocaleCode(code string) *ConfigProviderW {
	if l, ok := LocaleByCode(code); ok {
		p.cfg.locale = l
	}
	return p
}

// Defaults sets default properties of a component; keys merge with outer
// providers.
func (p *ConfigProviderW) Defaults(component string, props Props) *ConfigProviderW {
	if p.cfg.props == nil {
		p.cfg.props = map[string]Props{}
	}
	merged := Props{}
	for k, v := range p.cfg.props[component] {
		merged[k] = v
	}
	for k, v := range props {
		merged[k] = v
	}
	p.cfg.props[component] = merged
	return p
}

// ComponentDisabled disables every widget inside.
func (p *ConfigProviderW) ComponentDisabled(on bool) *ConfigProviderW { p.cfg.disabled = on; return p }

// Theme sets the theme of the scope. A provider called directly by the
// page function outside any other provider (the root provider) switches
// the window theme: background, global layers (Message, Notification) and
// everything built before it. A nested provider only themes the widgets
// built inside its Body, including their paint. The same applies to
// Flavor, Dark, PrimaryColor and Compact.
func (p *ConfigProviderW) Theme(th *theme.Theme) *ConfigProviderW { p.th = th; return p }

// Direction sets the inline direction of the scope: LTR, RTL, or DirAuto
// to derive it from the scope's Locale again (overriding an explicit
// direction of an outer provider). Without a provider that sets one, the
// direction follows the Locale, then SetDefaultDirection.
func (p *ConfigProviderW) Direction(d Direction) *ConfigProviderW { p.cfg.dir = d; return p }

// Flavor switches the design language (theme.Ant / theme.Element /
// theme.StyleShadcn) keeping
// dark mode and primary color settings of this provider.
func (p *ConfigProviderW) Flavor(f theme.Flavor) *ConfigProviderW { p.flavor = &f; return p }

// Dark switches between the light and dark algorithm.
func (p *ConfigProviderW) Dark(on bool) *ConfigProviderW { p.dark = &on; return p }

// PrimaryColor overrides the brand color (derived shades follow).
func (p *ConfigProviderW) PrimaryColor(col theme.Color) *ConfigProviderW { p.primary = &col; return p }

// Compact selects the compact size algorithm.
func (p *ConfigProviderW) Compact(on bool) *ConfigProviderW { p.compact = on; return p }

// HighContrast fixes the contrast of the scope: true draws
// theme.HighContrast of the scope's theme (WCAG AA for every required
// pair, docs/accessibility.md), false the standard variant. Without it
// the scope inherits: a theme rebuilt by Flavor, Dark, PrimaryColor or
// Compact stays high-contrast when the outer theme is (for example while
// uihost follows the system preference). Like Theme, the root provider
// applies it to the window; uihost then stops following the system
// theme for that window.
//
// HighContrast(false) inside a high-contrast scope regenerates the theme
// from its flavor, dark mode and component overrides, as Dark does: token
// customizations of the outer theme that the generator cannot reproduce
// are lost.
func (p *ConfigProviderW) HighContrast(on bool) *ConfigProviderW { p.hc = &on; return p }

type navThemeKey struct {
	flavor  theme.Flavor
	dark    bool
	primary theme.Color
	hasPri  bool
	compact bool
	hc      bool
	ovr     uint64 // hash of the outer theme's component overrides
}

// Components layers component token overrides (theme.Components; zero
// fields inherit) over the scope's theme. Repeated calls merge. Like Theme,
// it themes the window from the root provider and only the scope from a
// nested one.
func (p *ConfigProviderW) Components(cs theme.Components) *ConfigProviderW {
	if p.hasComps {
		p.comps = p.comps.Merge(cs)
	} else {
		p.comps, p.hasComps = cs, true
	}
	return p
}

type navCompsCache struct {
	base, th *theme.Theme
	hash     uint64
}

// scopedComponents applies the provider's component overrides to base,
// caching the derived theme while base and the overrides stay the same.
func (p *ConfigProviderW) scopedComponents(key ID, base *theme.Theme) *theme.Theme {
	if !p.hasComps {
		return base
	}
	h := p.comps.Hash()
	cache := State[navCompsCache](p.c, hashID(key, "components"), nil)
	// base == cache.th: the root provider's own output came back as the
	// window theme; layering the same overrides again changes nothing.
	if cache.th != nil && cache.hash == h && (cache.base == base || cache.th == base) {
		return cache.th
	}
	cache.base, cache.hash, cache.th = base, h, base.Override(p.comps)
	return cache.th
}

type navThemeCache struct {
	key navThemeKey
	th  *theme.Theme
}

// navHCCache caches theme.HighContrast(base) for HighContrast(true) over
// a given theme.
type navHCCache struct {
	base, th *theme.Theme
}

// highContrastOf returns the high-contrast variant of base, cached per
// provider while base stays the same.
func (p *ConfigProviderW) highContrastOf(key ID, base *theme.Theme) *theme.Theme {
	if base.HighContrast {
		return base
	}
	cache := State[navHCCache](p.c, hashID(key, "high-contrast"), nil)
	if cache.th == nil || cache.base != base {
		cache.base, cache.th = base, theme.HighContrast(base)
	}
	return cache.th
}

func (p *ConfigProviderW) resolveTheme(key ID) *theme.Theme {
	c := p.c
	if p.th != nil {
		if p.hc != nil && *p.hc {
			return p.highContrastOf(key, p.th)
		}
		return p.th
	}
	cur := c.Theme
	if p.flavor == nil && p.dark == nil && p.primary == nil && !p.compact {
		if p.hc == nil || *p.hc == cur.HighContrast {
			return nil
		}
		if *p.hc {
			return p.highContrastOf(key, cur)
		}
		// HighContrast(false) in a high-contrast scope: regenerate below.
	}
	k := navThemeKey{flavor: cur.Flavor, dark: cur.Dark, compact: p.compact, hc: cur.HighContrast}
	if p.flavor != nil {
		k.flavor = *p.flavor
	}
	if p.dark != nil {
		k.dark = *p.dark
	}
	if p.primary != nil {
		k.primary, k.hasPri = *p.primary, true
	}
	if p.hc != nil {
		k.hc = *p.hc
	}
	ovr := cur.Overrides()
	k.ovr = ovr.Hash()
	cache := State[navThemeCache](c, hashID(key, "theme"), nil)
	if cache.th != nil && cache.key == k {
		return cache.th
	}
	var opts []theme.Option
	if k.dark {
		opts = append(opts, theme.Dark())
	}
	if k.hasPri {
		opts = append(opts, theme.WithPrimary(k.primary))
	}
	if k.compact {
		opts = append(opts, theme.Compact())
	}
	// Keep the component overrides of the theme being rebuilt.
	opts = append(opts, theme.WithComponents(ovr))
	switch k.flavor {
	case theme.Element:
		cache.th = theme.ElementPlus(opts...)
	case theme.StyleShadcn:
		// Keep the base color and radius of a shadcn theme being toggled.
		sopt := theme.ShadcnOptions{}
		if cur.Shadcn != nil {
			sopt = cur.Shadcn.Options
			sopt.Dark, sopt.Compact = false, false
			if k.hasPri {
				sopt.Primary = theme.Color{}
			}
		}
		args := []theme.ShadcnArg{sopt}
		for _, o := range opts {
			args = append(args, o)
		}
		cache.th = theme.Shadcn(args...)
	default:
		cache.th = theme.AntDesign(opts...)
	}
	if k.hc {
		// Inherited or asked for: the rebuilt theme keeps the contrast.
		cache.th = theme.HighContrast(cache.th)
	}
	cache.key = k
	return cache.th
}

// Body builds fn inside the configuration scope.
func (p *ConfigProviderW) Body(fn func()) {
	c := p.c
	key := c.newID("config-provider", "")
	s := c.navConfigs()
	root := len(s.frames) == 0 && c.cur == c.root && c.env == c.rootEnvPtr()
	th := p.resolveTheme(key)
	if p.hasComps {
		if th == nil {
			th = c.Theme
		}
		th = p.scopedComponents(key, th)
	}
	if root && th != nil && th != c.Theme {
		// The root provider owns the window theme (background, global
		// layers, widgets built before it).
		c.Theme = th
		c.envRoot.th = th
		c.invalid = true // rebuild with the new tokens already in place
	}
	if th == nil {
		th = c.Theme
	}
	e := c.deriveEnv(key, th, resolveRTL(p.cfg.dir, p.cfg.locale), p.cfg.locale)
	prev, prevTh := c.enterEnv(e)
	if root {
		// The root provider's direction is the window's: the root node
		// (stamped before the page ran) mirrors its top-level children.
		c.root.ln.RTL = e.rtl
	}
	s.frames = append(s.frames, p.cfg)
	depth := len(s.frames)
	Disabled(c, p.cfg.disabled, fn)
	s = c.navConfigs()
	if len(s.frames) >= depth {
		s.frames = s.frames[:depth-1]
	}
	if root {
		prevTh = c.envRoot.th // keep a window theme set while fn ran
	}
	c.leaveEnv(prev, prevTh)
}
