package linux

// Pure logic of the desktop integration (displays, theme, shell, launcher
// badge), without build tags so it is tested on every host.

import (
	"fmt"
	"hash/fnv"
	"math"
	"net/url"
	"strconv"
	"strings"
)

// fileURI converts an absolute path to a file:// URI with the path
// percent-encoded.
func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}

// showItemsArgs are the (ass) arguments of org.freedesktop.FileManager1
// ShowItems in GVariant text format: the URIs to select and an empty
// startup notification id.
func showItemsArgs(path string) string {
	return "([" + gvString(fileURI(path)) + "], '')"
}

// portalReadArgs are the (ss) arguments of org.freedesktop.portal.Settings
// Read.
func portalReadArgs(namespace, key string) string {
	return "(" + gvString(namespace) + ", " + gvString(key) + ")"
}

// themeIsDark decides light/dark: the XDG portal's color-scheme when the
// desktop provides it (1 prefers dark, 2 prefers light, 0 has no
// preference), else GTK's settings — the prefer-dark flag or a theme named
// like "Adwaita-dark" / "Yaru:dark".
func themeIsDark(scheme uint32, haveScheme, preferDark bool, themeName string) bool {
	if haveScheme && scheme != 0 {
		return scheme == 1
	}
	if preferDark {
		return true
	}
	n := strings.ToLower(themeName)
	return strings.HasSuffix(n, "-dark") || strings.HasSuffix(n, ":dark") || strings.HasSuffix(n, "_dark")
}

// contrastIsHigh decides the high-contrast preference: the XDG portal's
// org.freedesktop.appearance contrast when the desktop provides it (1 asks
// for higher contrast, 0 has no preference; xdg-desktop-portal 1.19+,
// which GNOME 47+ feeds from org.gnome.desktop.a11y.interface
// high-contrast), else a GTK theme named like GNOME's "HighContrast" /
// "HighContrastInverse" (older GNOME, MATE, Xfce).
func contrastIsHigh(contrast uint32, haveContrast bool, themeName string) bool {
	if haveContrast && contrast == 1 {
		return true
	}
	n := strings.ToLower(themeName)
	return strings.Contains(n, "highcontrast") || strings.Contains(n, "high-contrast") || strings.Contains(n, "high contrast")
}

// portalAccent converts the portal's accent-color (r, g, b in [0, 1]) to
// 0xRRGGBBAA; any component outside the range means "unset" and gives 0.
func portalAccent(r, g, b float64) uint32 {
	var out uint32
	for _, v := range []float64{r, g, b} {
		if v < 0 || v > 1 || math.IsNaN(v) {
			return 0
		}
		out = out<<8 | uint32(math.Round(v*255))
	}
	return out<<8 | 0xff
}

// launcherCount parses a badge for the launcher API, which shows numbers
// only. A trailing "+" is dropped.
func launcherCount(text string) (int64, bool) {
	n, err := strconv.ParseInt(strings.TrimSuffix(text, "+"), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// launcherState is what the com.canonical.Unity.LauncherEntry Update signal
// carries; docks keep the last values they received for the app.
type launcherState struct {
	count    int64
	countOn  bool
	progress float64
	progOn   bool
	urgent   bool
}

// launcherPath is the object path the signal is sent from; any path works,
// one derived from the app URI keeps it stable.
func launcherPath(appURI string) string {
	h := fnv.New32a()
	h.Write([]byte(appURI))
	return fmt.Sprintf("/com/canonical/unity/launcherentry/%d", h.Sum32())
}

// launcherArgs are the (sa{sv}) arguments of Update in GVariant text format.
func launcherArgs(appURI string, s launcherState) string {
	p := s.progress
	if p < 0 || math.IsNaN(p) {
		p = 0
	}
	return fmt.Sprintf("(%s, {'count': <int64 %d>, 'count-visible': <%t>, 'progress': <double %s>, 'progress-visible': <%t>, 'urgent': <%t>})",
		gvString(appURI), s.count, s.countOn, strconv.FormatFloat(min(p, 1), 'f', -1, 64), s.progOn, s.urgent)
}
