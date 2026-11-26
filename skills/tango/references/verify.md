# Verify the running app

Use `tan dev` while iterating. It watches Go sources and runs the configured frontend dev server. For an embedded WebView build, run the project's frontend build before starting a Go build through MCP.

## Start in the background with MCP

For a client that reads `.mcp.json`, register the server from the app directory.

```json
{
  "mcpServers": {
    "tango": { "command": "tan", "args": ["mcp", "-root", ".", "-stdio"] }
  }
}
```

Call `tango_app_start` with the package directory relative to the MCP root. It builds with `tango_dev` and starts in the background. Pass an app-specific test data directory using the app's arguments or `env` entries, for example `["MY_APP_DATA_DIR=/path/to/test-data"]` when the app supports that variable. MCP reserves the `TANGO_` prefix. Use the returned `session` for subsequent calls.

For a native UI, inspect `tango_ui_tree`, choose the real semantic name and role, and operate it with `tango_ui_act`, `tango_ui_type`, or `tango_ui_key`. The visual label can differ from the semantic name, including when drawn text inserts spaces. For a WebView, use `tango_web_wait`, `tango_web_fill`, `tango_web_click`, and `tango_web_text` with CSS selectors. Use `tango_web_eval` for page state queries.

Capture either window with `tango_ui_screenshot` and call `tango_app_stop` after verification. Use `tango_app_output` to inspect logs. Native menu bars and tray items are outside the semantic tree. WebView key injection is unavailable through these tools. Use OS-level automation for native controls and WebView keyboard shortcuts. Build and start a proper macOS `.app` for notification permission and delivery. [macOS troubleshooting](pitfalls.md)

## Use HTTP from a shell

Start `tan mcp -root . -http -addr 127.0.0.1:7811` from the app directory. The server prints its URL and a generated bearer token. Put the token in `tango_mcp_token` in your local shell. Initialize at the printed URL. Copy `Mcp-Session-Id` from the response headers and `result.protocolVersion` from the JSON-RPC response. When streamed, the response is in the SSE `data:` line.

```sh
tango_mcp_url=http://127.0.0.1:7811
curl -siN "$tango_mcp_url" \
  -H "Authorization: Bearer $tango_mcp_token" \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  --data '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"shell","version":"1"}}}'
```

Set `tango_mcp_session` and `tango_mcp_protocol` to those values, then send the notification and start the app.

```sh
tango_rpc() {
  curl -sSN "$tango_mcp_url" \
    -H "Authorization: Bearer $tango_mcp_token" \
    -H "Mcp-Session-Id: $tango_mcp_session" \
    -H "MCP-Protocol-Version: $tango_mcp_protocol" \
    -H 'Content-Type: application/json' \
    -H 'Accept: application/json, text/event-stream' \
    --data "$1"
}
tango_rpc '{"jsonrpc":"2.0","method":"notifications/initialized"}'
tango_rpc '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"tango_app_start","arguments":{"target":"."}}}'
```

Pass the app session from the tool result as `arguments.session` to further tool calls. It is separate from the MCP protocol session header. [MCP tool schemas](https://github.com/tangoui/tango/blob/main/docs/mcp.md)
