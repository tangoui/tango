# Live UI documentation

Users chose directly manipulating controls in the documentation, rather than editing Go code in a browser. VitePress remains the site; the first release covers Button, Input, Switch, Slider, Select and Tabs, with usage, properties and Go snippets for each.

Render real Tango components in a Go js/wasm program, using the existing CPU renderer and ui.Window contract. Forward pointer, keyboard and committed/composing text input from the browser. Register a redistributable Go font so WASM can draw text without system fonts. An isolated iframe hosts each preview; Vue provides the surrounding document and theme/reset controls. Three Tango theme flavors and light/dark variants are selectable. Loading and startup errors must be visible and the frame must work under the GitHub Pages /tango/ base path. No server, native window, or external runtime service is required.

Alternatives: screenshot switching cannot demonstrate interaction; rebuilding controls in Vue does not show Tango rendering; a Go server cannot run on GitHub Pages. WASM with Canvas reuses the actual component code while retaining static hosting.

Keep the implementation under site/ and avoid changes to shared ui or native platform code. Existing gallery generation remains. Native-only capabilities are not included in browser demos. First-release scope does not promise a browser backend for full Tango apps or online Go compilation.

## Homepage direction

The user additionally requested that Go GUI be the main proposition. Lead both homepages with Go GUI and appearance, three theme flavors, the actual Go gallery dashboard screenshot, live component demos and a GUI template command. Retain WebView/hybrid capabilities and accurate experimental/platform scope.
