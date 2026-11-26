# WebView keyboard automation

`tango_web_key` takes the same `session`, `window` and `keys` fields as `tango_ui_key`. The selected WebView receives a physical key press and release with a chord's modifier bits. If a native menu consumes the press, the menu action takes precedence, as it does with keyboard input. This covers custom menu callbacks and standard Edit roles. A page can test its own keyboard handlers with an unbound chord.

## Protocol and dispatch

Add an additive `web_key` operation carrying the window ID and key chord in the existing input object. Keep the protocol version and existing canvas input unchanged. Parse the chord on the application side with `ParseKeys`, so CmdOrCtrl resolves for the app's platform. The server calls a development-only host entry point on the main thread. The hook checks the target surface and calls the optional native WebView keyboard capability. A completion callback allows WebView2 to report the outcome of its asynchronous input calls. The server waits for completion outside the main thread, with the existing automation timeout convention.

Malformed keys return `bad_request`. A closed or unknown window returns `no_window`. A canvas target, missing backend capability or key without a platform mapping returns `unsupported`. Backend failures return an error to the caller. A timeout can occur after dispatch began, so check the page or app state before retrying. The MCP result acknowledges dispatch. Wait for asynchronous page or app effects through `tango_web_wait`.

## Native delivery

- macOS uses AppKit NSEvents with the target window number and ANSI virtual key codes. Route the press through AppKit key equivalents so enabled NSMenu items invoke their existing handlers. Resolve responder-chain menu actions against the target window and deliver the remaining events to WKWebView without activating the application.
- Windows uses the WebView2 accelerator plan and the normal command dispatcher for matching Ctrl, Alt and Shift chords. Edit commands run against the target WebView through the Chromium DevTools input path. Other chords use two `Input.dispatchKeyEvent` calls with the DOM code, Windows virtual key, modifier bits and printable text. Both calls complete before the request returns.
- Linux uses a GdkEventKey with keyval, hardware code and modifier state. Give the target GtkWindow its normal accelerator handling first. Deliver the remaining events directly to WebKitGTK, reusing the native GDK event API already used for WebView input.

Only the selected app window and WebView are targeted. Window-local responder or widget focus may be set to reach the WebView. Delivery does not post system-wide events, move the pointer or activate the application. Modifier bits describe one chord and require no global keyboard state changes. Printable chord characters follow the US key mapping used by Tango's key enum. IME composition and system shortcuts are outside this API.

## Verification

Run protocol and MCP tests for request shape, window targeting, parsing and error propagation. Exercise native key mappings and menu precedence in platform-independent tests. Build an automated WebView fixture with DOM event listeners, editable controls and a CmdOrCtrl+S menu callback. Run it from a desktop session using MCP and record event values, input values, the menu callback, foreground identity and a WebView screenshot. Include Edit role coverage. The same fixture can run on Windows in an interactive desktop session and on Linux under a GTK3 and WebKitGTK-enabled Xvfb session. Run Go tests for both modules, cross-platform vet, formatting and the API checker.
