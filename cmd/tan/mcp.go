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
	"sync"
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

	// ctx is done once close (or kill) has started; handlers that start
	// children merge it into their own context (see track).
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	closing   bool           // guards inflight.Add
	inflight  sync.WaitGroup // tracked handlers still running
	closeOnce sync.Once
}

// errShuttingDown is the error of tools called while tan mcp shuts down.
var errShuttingDown = errors.New("tan mcp: the server is shutting down")

// mcpExit ends the process after a second signal; tests replace it.
var mcpExit = os.Exit

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
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.addProjectTools()
	s.addUITools()
	s.addWebTools()
	return s, nil
}

// track registers a handler that starts children (an app being launched,
// tan init) so that close waits for it, and returns its context merged
// with the server's: shutting down cancels it, which kills what it
// started. done must be called when the handler returns. After close has
// started it refuses.
func (s *mcpServer) track(ctx context.Context) (_ context.Context, done func(), err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closing {
		return nil, nil, errShuttingDown
	}
	s.inflight.Add(1)
	ctx, cancel := context.WithCancelCause(ctx)
	stop := context.AfterFunc(s.ctx, func() { cancel(errShuttingDown) })
	return ctx, func() {
		stop()
		cancel(nil)
		s.inflight.Done()
	}, nil
}

// startClosing refuses new tracked handlers and cancels the running ones.
func (s *mcpServer) startClosing() {
	s.mu.Lock()
	s.closing = true
	s.mu.Unlock()
	s.cancel()
}

// close stops every job and app the server started, and waits for the
// tracked handlers, whose launches it cancels. Later calls wait for the
// first one to finish.
func (s *mcpServer) close() {
	s.closeOnce.Do(func() {
		s.startClosing()
		var wg sync.WaitGroup
		wg.Go(s.jobs.stopAll)
		wg.Go(s.apps.closeAll)
		wg.Wait()
		s.inflight.Wait()
	})
}

// kill ends every child at once (a second signal while close runs): the
// jobs' process groups and the started apps' processes. It waits a moment
// for the cancelled launches to kill theirs.
func (s *mcpServer) kill() {
	s.startClosing()
	s.jobs.killAll()
	s.apps.killAll()
	done := make(chan struct{})
	go func() {
		s.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
	}
}

// serveUntilSignal runs serve until it returns or the first signal comes
// in sigs, which cancels its context; then it closes s, stopping every
// job and app it started. A second signal while that runs kills them all
// at once and exits.
func (s *mcpServer) serveUntilSignal(sigs <-chan os.Signal, serve func(context.Context) error) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	served := make(chan error, 1)
	go func() { served <- serve(ctx) }()
	var err error
	signalled := false
	select {
	case err = <-served:
	case <-sigs:
		signalled = true
		cancel()
	}
	// Cleanup starts now, while serve winds down: Conn.Close fails the UI
	// requests in flight, and the server context cancels launches.
	closed := make(chan struct{})
	go func() {
		s.close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-sigs:
		fmt.Fprintln(os.Stderr, "tan mcp: second signal: killing the apps and jobs")
		s.kill()
		mcpExit(1)
		return errors.New("tan mcp: killed by a second signal")
	}
	if signalled {
		<-served
		return nil
	}
	return err
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
	defer s.close() // serveUntilSignal has closed it, unless listening failed

	// The handler stays installed until cleanup is over: a second signal
	// kills the children instead of tan mcp alone.
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigs)
	if !*useHTTP {
		return s.serveUntilSignal(sigs, func(ctx context.Context) error {
			// Run returns nil when stdin reaches EOF and ctx.Err() on a signal.
			if err := s.srv.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
				return err
			}
			return nil
		})
	}
	ln, err := listenLoopback(*addr)
	if err != nil {
		return err
	}
	return s.serveUntilSignal(sigs, func(ctx context.Context) error { return s.serveHTTP(ctx, ln, *token) })
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
