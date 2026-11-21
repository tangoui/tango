package devhost

import (
	"encoding/gob"
	"net/http"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

type EmbedOpen struct {
	Req, Win int
	View     uint64
	Opts     platform.SurfaceOptions
	Web      platform.WebOptions
}

type EmbedOpened struct {
	Req         int
	Err         string
	Unsupported bool
}

type EmbedOp struct {
	Win       int
	View      uint64
	Op        string
	Text      string
	Number    float64
	Req       int
	Placement platform.WebPlacement
}

type EmbedEvent struct {
	Win             int
	View            uint64
	Event           platform.SurfaceEvent
	Origin, Message string
	NavKind         platform.WebNavigationKind
	NavToken        uint64
	NavPage         platform.WebPageState
	NavError        string
}

type EmbedResult struct {
	Win         int
	View        uint64
	Req         int
	PNG         []byte
	Err         string
	Moved       bool
	Unsupported bool
}

type EmbedAssetRequest struct {
	Req, Win    int
	View        uint64
	Method, URL string
	Header      http.Header
	Body        []byte
}

type EmbedAssetResponse struct {
	Req, Status int
	Header      http.Header
	Body        []byte
}

type Activity struct {
	Win int
	At  time.Time
}

func init() {
	for _, m := range []any{EmbedOpen{}, EmbedOpened{}, EmbedOp{}, EmbedEvent{}, EmbedResult{}, EmbedAssetRequest{}, EmbedAssetResponse{}, Activity{}} {
		gob.Register(m)
	}
}
