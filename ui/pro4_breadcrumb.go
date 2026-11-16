package ui

// ProBreadcrumbW is the route-aware Breadcrumb exported by ProLayout.
// It delegates rendering, keyboard handling and semantics to BreadcrumbW.
type ProBreadcrumbW struct {
	_ [0]func()
	*BreadcrumbW
}

// ProBreadcrumb starts a breadcrumb from the innermost ProLayout's route.
// Items overrides the route items, including with an empty slice.
// ConfigProvider.Defaults("ProBreadcrumb", ...): "separator" (string).
func ProBreadcrumb(c *Context) *ProBreadcrumbW {
	items := RouteContext(c).BreadcrumbItems
	b := BreadcrumbItems(c, items)
	b.sep = Prop(c, "ProBreadcrumb", "separator", b.sep)
	return &ProBreadcrumbW{BreadcrumbW: b}
}

// Items replaces the breadcrumb items (rather than appending).
func (b *ProBreadcrumbW) Items(items []BreadcrumbItem) *ProBreadcrumbW {
	b.items = items
	return b
}

// Separator replaces the separator between items.
func (b *ProBreadcrumbW) Separator(s string) *ProBreadcrumbW {
	b.BreadcrumbW.Separator(s)
	return b
}

// Len reports the number of breadcrumb items.
func (b *ProBreadcrumbW) Len() int { return len(b.items) }

// Show builds the breadcrumb. Empty route breadcrumbs produce no landmark.
func (b *ProBreadcrumbW) Show() *ProBreadcrumbW {
	if len(b.items) > 0 {
		b.BreadcrumbW.Show()
	}
	return b
}
