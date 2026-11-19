package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpServer is the MCP server behind tan mcp: the tools act on the project
// under root and on the apps and jobs they start.
type mcpServer struct {
	root string // absolute, symlinks resolved
	srv  *mcp.Server
	jobs *jobManager
	apps *appManager
}

// appManager holds the app sessions the UI tools started or attached to.
// Placeholder: replaced with the real session manager.
type appManager struct{}

func newAppManager() *appManager { return &appManager{} }

// closeAll ends every app session.
func (*appManager) closeAll() {}

// newMCPServer returns the server for the project directory root.
func newMCPServer(root string) (*mcpServer, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("-root: %w", err)
	}
	if fi, err := os.Stat(real); err != nil {
		return nil, fmt.Errorf("-root: %w", err)
	} else if !fi.IsDir() {
		return nil, fmt.Errorf("-root %s: not a directory", root)
	}
	s := &mcpServer{
		root: real,
		srv:  mcp.NewServer(&mcp.Implementation{Name: "tango", Version: version}, nil),
		jobs: newJobManager(),
		apps: newAppManager(),
	}
	s.addProjectTools()
	return s, nil
}

// close stops every job and app the server started.
func (s *mcpServer) close() {
	s.jobs.stopAll()
	s.apps.closeAll()
}

func runMCP(args []string) error {
	fs := newFlags("mcp", "[-root dir] [-stdio | -http [-addr addr] [-token token]]")
	root := fs.String("root", ".", "project `dir` the tools work in; paths outside it are refused")
	stdio := fs.Bool("stdio", false, "serve MCP over standard input and output (the default)")
	useHTTP := fs.Bool("http", false, "serve MCP over streamable HTTP on a loopback address instead of stdio")
	addr := fs.String("addr", "127.0.0.1:0", "with -http: loopback `address` to listen on (127.0.0.1, localhost or [::1])")
	token := fs.String("token", "", "with -http: bearer `token` clients must send (default: a random one, printed to stderr)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := noArgs(fs); err != nil {
		return err
	}
	if *stdio && *useHTTP {
		return errors.New("-stdio and -http cannot be used together")
	}
	if !*useHTTP {
		var httpOnly bool
		fs.Visit(func(f *flag.Flag) { httpOnly = httpOnly || f.Name == "addr" || f.Name == "token" })
		if httpOnly {
			return errors.New("-addr and -token need -http")
		}
	}
	if *useHTTP {
		if err := checkLoopbackAddr(*addr); err != nil {
			return err
		}
	}
	s, err := newMCPServer(*root)
	if err != nil {
		return err
	}
	defer s.close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if !*useHTTP {
		// Run returns nil when stdin reaches EOF and ctx.Err() on a signal.
		if err := s.srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
			return err
		}
		return nil
	}
	ln, err := listenLoopback(*addr)
	if err != nil {
		return err
	}
	return s.serveHTTP(ctx, ln, *token)
}

// listenLoopback listens on addr, which checkLoopbackAddr accepted.
func listenLoopback(addr string) (net.Listener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	// localhost is resolved by the system: make sure it gave a loopback address.
	if ta, ok := ln.Addr().(*net.TCPAddr); !ok || !ta.IP.IsLoopback() {
		ln.Close()
		return nil, fmt.Errorf("-addr %s: listening on %s, not a loopback address", addr, ln.Addr())
	}
	return ln, nil
}

// serveHTTP serves the streamable HTTP transport on the loopback listener
// ln until ctx is done. Requests must carry "Authorization: Bearer token";
// without a token a random one is made. The address and token go to stderr.
func (s *mcpServer) serveHTTP(ctx context.Context, ln net.Listener, token string) error {
	if token == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			ln.Close()
			return err
		}
		token = hex.EncodeToString(b)
	}
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s.srv }, nil)
	hs := &http.Server{Handler: httpGuard(token, handler), ReadHeaderTimeout: 10 * time.Second}
	fmt.Fprintf(os.Stderr, "tan mcp: listening on http://%s (token %s)\n", ln.Addr(), token)

	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		// Event streams stay open: give requests a moment, then cut them.
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if hs.Shutdown(sctx) != nil {
			hs.Close()
		}
	}()
	err := hs.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		<-done
		return nil
	}
	return err
}

// checkLoopbackAddr accepts only listen addresses on 127.0.0.1, localhost
// or [::1]: tan mcp is not reachable from other machines.
func checkLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("-addr %q: %w", addr, err)
	}
	switch host {
	case "127.0.0.1", "localhost", "::1":
		return nil
	}
	return fmt.Errorf("-addr %q: only loopback addresses (127.0.0.1, localhost, [::1]) are allowed", addr)
}

// httpGuard passes on requests that carry "Authorization: Bearer token"
// (scheme in any case; 401 otherwise) and whose Host, and Origin when present, are loopback
// (403 otherwise; a page on another site reaching the port through DNS
// rebinding sends its own Host and Origin).
func httpGuard(token string, next http.Handler) http.Handler {
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, got, _ := strings.Cut(r.Header.Get("Authorization"), " ")
		if token == "" || !strings.EqualFold(scheme, "Bearer") || subtle.ConstantTimeCompare([]byte(got), want) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !loopbackOrigin(o) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		if !loopbackHost(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// loopbackOrigin reports whether an Origin header names a loopback host.
func loopbackOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false // includes "null"
	}
	return loopbackHost(u.Host)
}

// loopbackHost reports whether host, with or without a port, is localhost
// or a loopback IP.
func loopbackHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
