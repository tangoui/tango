package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// frontendStartup monitors the server through the first application launch.
// Only the session goroutine applies the URL and starts app processes.
type frontendStartup struct {
	srv    *parallelDevServer
	ctx    context.Context
	cancel context.CancelCauseFunc
	done   chan struct{}
	ready  chan struct{}
	url    string // published by closing ready
}

type reportedFrontendURL struct {
	url string
	at  time.Time
}

type parallelDevServer struct {
	*process
	url      chan reportedFrontendURL
	deadline time.Time
}

func watchFrontend(ctx context.Context, srv *parallelDevServer, timer *time.Timer) *frontendStartup {
	ctx, cancel := context.WithCancelCause(ctx)
	s := &frontendStartup{srv: srv, ctx: ctx, cancel: cancel, ready: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer timer.Stop()
		var report reportedFrontendURL
		select {
		case report = <-srv.url:
		case <-srv.done:
			s.cancel(s.exitError())
			return
		case <-timer.C:
			// If the scanner already delivered a URL, accept it even if
			// the monitor was not scheduled until the timer was ready.
			select {
			case report = <-srv.url:
			default:
				s.cancel(errDevURLTimeout)
				return
			}
		case <-ctx.Done():
			return
		}
		if report.at.After(srv.deadline) {
			s.cancel(errDevURLTimeout)
			return
		}
		timer.Stop()
		s.url = report.url
		close(s.ready)
		select {
		case <-srv.done:
			s.cancel(s.exitError())
		case <-ctx.Done():
		}
	}()
	return s
}

func (s *frontendStartup) close() {
	s.cancel(nil)
	<-s.done
}

func (s *frontendStartup) exitError() error {
	const message = "the front-end dev server exited before the app started"
	if s.srv.err != nil {
		return fmt.Errorf("%s: %w", message, s.srv.err)
	}
	return fmt.Errorf("%s", message)
}

func (s *frontendStartup) failure() error {
	// A process exit may be observable before the monitor sees it. Give
	// that exit priority over a URL that is also ready.
	select {
	case <-s.srv.done:
		return s.exitError()
	default:
		return context.Cause(s.ctx)
	}
}

func (s *frontendStartup) waitFrontend(ctx context.Context) (string, error) {
	select {
	case <-s.ready:
	case <-s.ctx.Done():
	case <-ctx.Done():
	}
	if err := s.failure(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return s.url, nil
}

func startParallelDevServer(dir, line string, out io.Writer) (*parallelDevServer, func(), error) {
	cmd := shellCommand(line)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "FORCE_COLOR=1")
	// Give the command a real file descriptor so Wait observes the shell's
	// exit even if a descendant still holds its stdout or stderr open.
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdout, cmd.Stderr = w, w
	group, err := startDevGroup(cmd)
	deadline := time.Now().Add(devURLTimeout)
	w.Close()
	if err != nil {
		r.Close()
		return nil, nil, err
	}
	p := waitProcess(cmd)
	srv := &parallelDevServer{process: p, url: make(chan reportedFrontendURL, 1), deadline: deadline}
	// Keep the scanner interruptible even if the consumer of stdout stops
	// reading. Closing the pipe releases the scanner and ends the copy
	// once any write already in progress returns.
	logs, logWriter := io.Pipe()
	go func() {
		defer logs.Close()
		_, _ = io.Copy(out, logs)
	}()
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		defer logWriter.Close()
		scanDevOutput(r, logWriter, func(url string) {
			srv.url <- reportedFrontendURL{url: url, at: time.Now()}
		}, true)
	}()
	stop := func() {
		if groupOutlivesLeader {
			p.stop(3 * time.Second)
		} else {
			group.kill()
			<-p.done
		}
		group.release()
		r.Close()
		logWriter.Close()
		<-scanned
	}
	return srv, stop, nil
}

func runStartupCommand(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// In hot mode os/exec copies compiler output from a pipe. If the
	// compiler exits but a child holds the pipe, bound that final drain so
	// we can clean up the group and report the result of the attempt.
	cmd.WaitDelay = 100 * time.Millisecond
	group, err := startDevGroup(cmd)
	if err != nil {
		return err
	}
	defer group.release()
	p := waitProcess(cmd)
	select {
	case <-ctx.Done():
		group.kill()
		<-p.done
		return ctx.Err()
	case <-p.done:
		// A failed compiler or wrapper can leave descendants behind.
		group.kill()
		return p.err
	}
}
