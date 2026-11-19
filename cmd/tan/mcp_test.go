package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCheckLoopbackAddr(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "localhost:8080", "[::1]:9"} {
		if err := checkLoopbackAddr(addr); err != nil {
			t.Errorf("checkLoopbackAddr(%q) = %v, want nil", addr, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:1", "192.168.1.2:1", "example.com:1", ":8080", "127.0.0.1"} {
		if err := checkLoopbackAddr(addr); err == nil {
			t.Errorf("checkLoopbackAddr(%q) = nil, want an error", addr)
		}
	}
}

// guardedServer serves httpGuard(token, ok) on a loopback test server.
func guardedServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	ts := httptest.NewServer(httpGuard(token, ok))
	t.Cleanup(ts.Close)
	return ts
}

// status sends a GET to ts with the given headers ("Host" sets the request's
// Host) and returns the response status.
func status(t *testing.T, ts *httptest.Server, headers map[string]string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ts.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestHTTPRequiresToken(t *testing.T) {
	ts := guardedServer(t, "s3cret")
	for _, c := range []struct {
		auth string
		want int
	}{
		{"", http.StatusUnauthorized},
		{"Bearer wrong", http.StatusUnauthorized},
		{"Bearer s3cret", http.StatusOK},
	} {
		h := map[string]string{}
		if c.auth != "" {
			h["Authorization"] = c.auth
		}
		if got := status(t, ts, h); got != c.want {
			t.Errorf("Authorization %q: status %d, want %d", c.auth, got, c.want)
		}
	}
}

func TestHTTPRejectsForeignOrigin(t *testing.T) {
	ts := guardedServer(t, "s3cret")
	auth := "Bearer s3cret"
	for _, c := range []struct {
		name    string
		headers map[string]string
		want    int
	}{
		{"foreign origin", map[string]string{"Authorization": auth, "Origin": "https://evil.example"}, http.StatusForbidden},
		{"null origin", map[string]string{"Authorization": auth, "Origin": "null"}, http.StatusForbidden},
		{"loopback origin", map[string]string{"Authorization": auth, "Origin": "http://127.0.0.1:5173"}, http.StatusOK},
		{"foreign host", map[string]string{"Authorization": auth, "Host": "evil.example"}, http.StatusForbidden},
		{"localhost host", map[string]string{"Authorization": auth, "Host": "localhost:1234"}, http.StatusOK},
	} {
		if got := status(t, ts, c.headers); got != c.want {
			t.Errorf("%s: status %d, want %d", c.name, got, c.want)
		}
	}
}

// TestHTTPGuardEdgeCases calls the guard directly, so Host can be anything
// (including empty).
func TestHTTPGuardEdgeCases(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	guard := httpGuard("s3cret", ok)
	for _, c := range []struct {
		host, origin, auth string
		want               int
	}{
		{"localhost.evil.com", "", "Bearer s3cret", http.StatusForbidden},
		{"localhost.evil.com:80", "", "Bearer s3cret", http.StatusForbidden},
		{"", "", "Bearer s3cret", http.StatusForbidden},
		{"127.0.0.1.evil.com", "", "Bearer s3cret", http.StatusForbidden},
		{"[::1]:9", "", "Bearer s3cret", http.StatusOK},
		{"127.0.0.1:9", "http://localhost.evil.com", "Bearer s3cret", http.StatusForbidden},
		{"127.0.0.1:9", "null", "Bearer s3cret", http.StatusForbidden},
		{"127.0.0.1:9", "http://localhost:5173", "Bearer s3cret", http.StatusOK},
		{"127.0.0.1:9", "", "bearer s3cret", http.StatusOK},
		{"127.0.0.1:9", "", "BEARER s3cret", http.StatusOK},
		{"127.0.0.1:9", "", "Basic s3cret", http.StatusUnauthorized},
		{"127.0.0.1:9", "", "Bearer", http.StatusUnauthorized},
		{"127.0.0.1:9", "", "Bearer ", http.StatusUnauthorized},
		{"127.0.0.1:9", "", "s3cret", http.StatusUnauthorized},
	} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Host = c.host
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if c.auth != "" {
			r.Header.Set("Authorization", c.auth)
		}
		w := httptest.NewRecorder()
		guard.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("Host %q Origin %q Authorization %q: status %d, want %d", c.host, c.origin, c.auth, w.Code, c.want)
		}
	}
	// An empty token accepts nothing.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Host = "127.0.0.1:9"
	r.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	httpGuard("", ok).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("empty token: status %d, want 401", w.Code)
	}
}

func TestMCPInitializeListsServer(t *testing.T) {
	s, err := newMCPServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	if got := cs.InitializeResult().ServerInfo.Name; got != "tango" {
		t.Errorf("ServerInfo.Name = %q, want tango", got)
	}
}

func TestMCPFlagsExclusive(t *testing.T) {
	if err := runMCP([]string{"-stdio", "-http"}); err == nil {
		t.Fatal("runMCP(-stdio -http) = nil, want an error")
	}
	for _, args := range [][]string{{"-addr", "127.0.0.1:1"}, {"-stdio", "-token", "x"}} {
		if err := runMCP(args); err == nil || !strings.Contains(err.Error(), "need -http") {
			t.Errorf("runMCP(%q) = %v, want the need -http error", args, err)
		}
	}
}

func TestMCPServerRootIsResolved(t *testing.T) {
	if _, err := newMCPServer(t.TempDir() + "/missing"); err == nil {
		t.Error("newMCPServer(missing dir) = nil error")
	}
}

// bearer adds an Authorization header to every request.
type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

func TestMCPHTTPInitialize(t *testing.T) {
	s, err := newMCPServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.close()
	ln, err := listenLoopback("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- s.serveHTTP(ctx, ln, "t0k") }()
	endpoint := "http://" + ln.Addr().String()

	connect := func(token string) (*mcp.ClientSession, error) {
		tr := &mcp.StreamableClientTransport{
			Endpoint:   endpoint,
			HTTPClient: &http.Client{Transport: bearer{token, http.DefaultTransport}},
			MaxRetries: -1,
		}
		return mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, tr, nil)
	}
	if cs, err := connect("wrong"); err == nil {
		cs.Close()
		t.Error("connect with a wrong token succeeded")
	}
	cs, err := connect("t0k")
	if err != nil {
		t.Fatal(err)
	}
	if got := cs.InitializeResult().ServerInfo.Name; got != "tango" {
		t.Errorf("ServerInfo.Name = %q, want tango", got)
	}
	cs.Close()

	cancel()
	if err := <-served; err != nil {
		t.Errorf("serveHTTP after cancel = %v, want nil", err)
	}
}
