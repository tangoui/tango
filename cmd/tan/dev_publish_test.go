package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDevPublicationLockCoversStartAndWaitCanBeCanceled(t *testing.T) {
	d := newDevTest(t, "")
	s := &devSession{cfg: d.cfg, out: d.out}
	a, b := filepath.Join(d.cfg.dir, "a"), filepath.Join(d.cfg.dir, "b")
	writeDevTestFile(t, a, "#!/bin/sh\necho A >> \"$TAN_TEST_RUNS\"\n", 0o755)
	writeDevTestFile(t, b, "#!/bin/sh\necho B >> \"$TAN_TEST_RUNS\"\n", 0o755)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	starting, resume := make(chan struct{}), make(chan struct{})
	cmd := exec.CommandContext(devStartBarrier{Context: ctx, starting: starting, resume: resume}, d.out)
	cmd.Cancel = nil
	done := make(chan error, 1)
	go func() {
		p, err := s.publishAndStart(ctx, a, false, func() *exec.Cmd { return cmd }, nil)
		if err == nil {
			<-p.done
			err = p.err
		}
		done <- err
	}()
	defer func() { close(resume) }()
	select {
	case <-starting:
	case err := <-done:
		t.Fatalf("start returned before reaching the barrier: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	short, cancelWait := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancelWait()
	p, err := s.publishAndStart(short, b, false, func() *exec.Cmd { return exec.CommandContext(short, d.out) }, nil)
	if p != nil {
		<-p.done
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("contender returned %v while A held the lock", err)
	}
	content, err := os.ReadFile(d.out)
	if err != nil || string(content) != "#!/bin/sh\necho A >> \"$TAN_TEST_RUNS\"\n" {
		t.Fatalf("contender changed the published target: %q (%v)", content, err)
	}
	stages, err := filepath.Glob(filepath.Join(devDir(d.out), ".publish-*"))
	if err != nil || len(stages) != 1 {
		t.Fatalf("waiting contender left a stage behind: %v (%v)", stages, err)
	}
	// Release A's Start and assert the stable executable it actually ran.
	resume <- struct{}{}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if runs := devTestLines(t, d.runs); len(runs) != 1 || runs[0] != "A" {
		t.Errorf("executed %q, want only A", runs)
	}
}

// Done is called by exec.Cmd.Start just before starting the process.
type devStartBarrier struct {
	context.Context
	starting, resume chan struct{}
}

func (c devStartBarrier) Done() <-chan struct{} {
	close(c.starting)
	<-c.resume
	return c.Context.Done()
}
