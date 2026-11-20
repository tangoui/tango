package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDevParallelReadinessPrefersObservedExit(t *testing.T) {
	exited := make(chan struct{})
	srv := frontendReadinessFixture(exited)
	srv.url <- reportedFrontendURL{url: "http://localhost:4321/", at: time.Now()}
	exitErr := errors.New("exit status 23")
	srv.err = exitErr
	close(exited)
	startup := watchFrontend(t.Context(), srv, time.NewTimer(time.Minute))
	defer startup.close()
	if u, err := startup.waitFrontend(t.Context()); u != "" || !errors.Is(err, exitErr) {
		t.Fatalf("waitFrontend = %q, %v, want the already observable exit", u, err)
	}
}

func TestDevParallelBuildContinuesBeyondURLDeadline(t *testing.T) {
	exited := make(chan struct{})
	srv := frontendReadinessFixture(exited)
	srv.url <- reportedFrontendURL{url: "http://localhost:4321/", at: time.Now()}
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	startup := watchFrontend(ctx, srv, timer)
	defer startup.close()
	if u, err := startup.waitFrontend(ctx); err != nil || u != "http://localhost:4321/" {
		t.Fatalf("waitFrontend = %q, %v", u, err)
	}
	// Make the deadline signal available after readiness. Repeated waits
	// must see the same URL, and frontend exit must still cancel startup.
	timer.Reset(0)
	if u, err := startup.waitFrontend(ctx); err != nil || u != "http://localhost:4321/" {
		t.Fatalf("waitFrontend after readiness = %q, %v", u, err)
	}
	select {
	case <-startup.ctx.Done():
		t.Fatalf("deadline still affected startup: %v", startup.failure())
	case <-timer.C:
	}
	exitErr := errors.New("exit status 24")
	srv.err = exitErr
	close(exited)
	<-startup.ctx.Done()
	if !errors.Is(context.Cause(startup.ctx), exitErr) {
		t.Fatalf("startup cause = %v, want the frontend exit after URL readiness", context.Cause(startup.ctx))
	}
}

func TestDevParallelRejectsURLReportedAfterDeadline(t *testing.T) {
	srv := frontendReadinessFixture(make(chan struct{}))
	srv.deadline = time.Now().Add(-time.Second)
	srv.url <- reportedFrontendURL{url: "http://localhost:4321/", at: time.Now()}
	startup := watchFrontend(t.Context(), srv, time.NewTimer(0))
	defer startup.close()
	if url, err := startup.waitFrontend(t.Context()); url != "" || !errors.Is(err, errDevURLTimeout) {
		t.Fatalf("waitFrontend with late URL = %q, %v, want missing URL timeout", url, err)
	}
}

func frontendReadinessFixture(exited chan struct{}) *parallelDevServer {
	return &parallelDevServer{
		process:  &process{done: exited},
		url:      make(chan reportedFrontendURL, 1),
		deadline: time.Now().Add(time.Minute),
	}
}
