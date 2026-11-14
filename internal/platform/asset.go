package platform

import "errors"

// ErrResponseFinished is the error a backend's asset response writer (the
// http.ResponseWriter handed to Host.ServeAsset) returns from Write and
// FlushError once the response is complete: the handler returned and the
// end of the response was queued for the WebView. darwin and ios use it, so
// a late write fails the same way on both.
var ErrResponseFinished = errors.New("tango: asset response already finished")
