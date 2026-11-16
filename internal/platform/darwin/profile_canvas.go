//go:build darwin && !ios && !tango_backend_webview && tango_backend_canvas
package darwin
const ( profileWebViewEnabled=false; profileCanvasEnabled=true; profileExpectedWebView=false; profileExpectedCanvas=true )
