package ui

import "github.com/tangoui/tango/ui/theme"

// ThemeFor returns the theme component k reads: the current theme with k's
// Alias overrides applied (theme.Components.X.Alias), or c.Theme itself when
// k has none. Components capture it when they are built and paint from it.
func (c *Context) ThemeFor(k theme.Comp) *theme.Theme { return c.Theme.For(k) }

// Token returns the resolved component tokens of the current theme.
func (c *Context) Token() *theme.Components { return &c.Theme.Components }
