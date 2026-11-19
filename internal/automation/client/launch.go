package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
)

// EnvLaunchBackground is set to 1 for a launched program: on macOS it
// starts without activating and shows its windows behind the others.
// Passing EnvLaunchBackground+"=0" in the extra environment launches it in
// front.
const EnvLaunchBackground = "TANGO_LAUNCH_BACKGROUND"

// devTag links the automation channel into a build.
const devTag = "tango_dev"

// LaunchOptions configure Launch.
type LaunchOptions struct {
	// Target is a Go package (".", "./examples/ledger", an import path),
	// built with -tags tango_dev, or the path of an executable built so.
	Target string
	// Dir is the working directory of the build and the program ("" the
	// current one); a relative executable Target is resolved against it.
	Dir string
	// Args are the program's arguments.
	Args []string
	// Env adds KEY=VALUE entries to the inherited environment.
	Env []string
	// BuildFlags are extra `go build` flags when Target is a package.
	BuildFlags []string
	// Addr is the TANGO_AUTOMATION value; "" means "127.0.0.1:0" (a free
	// loopback port).
	Addr string
	// Output receives the program's stdout and stderr (nil discards them).
	Output io.Writer
	// StartTimeout bounds the wait for the program to open its control
	// channel after it started (the build is not counted); 0 means 30s.
	StartTimeout time.Duration
	// TempDir receives the build and the token file; it is required and
	// left to the caller to remove.
	TempDir string
}

// Process is a program started by Launch and connected to.
type Process struct {
	Conn *Conn
	Cmd  *exec.Cmd
	// Channel is the address and token the program wrote to its token
	// file: a second connection can be dialed with them.
	Channel wire.TokenFile

	exited chan struct{}
	runErr error // set before exited is closed
}

// Launch builds target if it is a package, starts it with the automation
// channel on and connects to it. On error nothing is left running.
func Launch(o LaunchOptions) (*Process, error) {
	return LaunchContext(context.Background(), o)
}

// LaunchContext is Launch, given up when ctx is done: the build is
// stopped, or the started program killed, and ctx's error returned. A
// done ctx does not affect the program once LaunchContext has returned.
func LaunchContext(ctx context.Context, o LaunchOptions) (*Process, error) {
	if o.TempDir == "" {
		return nil, errors.New("LaunchOptions.TempDir is empty")
	}
	bin, err := build(ctx, o.Target, o.Dir, o.BuildFlags, o.TempDir)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	tokenFile := filepath.Join(o.TempDir, "automation.json")
	addr := o.Addr
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	cmd := exec.Command(bin, o.Args...)
	cmd.Dir = o.Dir
	cmd.Env = LaunchEnv(os.Environ(), o.Env, addr, tokenFile)
	cmd.Stdout, cmd.Stderr = o.Output, o.Output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %v", bin, err)
	}
	p := &Process{Cmd: cmd, exited: make(chan struct{})}
	go func() {
		p.runErr = cmd.Wait()
		close(p.exited)
	}()
	start := o.StartTimeout
	if start <= 0 {
		start = 30 * time.Second
	}
	tf, err := waitTokenFile(ctx, tokenFile, p.exited, p.ExitErr, start)
	p.Channel = tf
	if err == nil {
		if p.Conn, err = Dial(tf.Addr, tf.Token); err != nil {
			err = fmt.Errorf("connect to %s: %v", o.Target, err)
		}
	}
	if err != nil {
		cmd.Process.Kill()
		<-p.exited
		return nil, err
	}
	return p, nil
}

// Exited is closed when the program has exited.
func (p *Process) Exited() <-chan struct{} { return p.exited }

// ExitErr returns cmd.Wait's result once the program has exited, nil
// before.
func (p *Process) ExitErr() error {
	select {
	case <-p.exited:
		return p.runErr
	default:
		return nil
	}
}

// Quit asks the app to quit (unless it has exited), closes the connection
// and waits for the program to exit. If it has not exited when timeout
// has passed since the call (the quit request included), it is killed and
// Quit reports so.
func (p *Process) Quit(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	select {
	case <-p.exited:
	default:
		p.Conn.Do(&wire.Request{Op: wire.OpQuit}, timeout)
	}
	p.Conn.Close()
	select {
	case <-p.exited:
		return nil
	case <-time.After(time.Until(deadline)):
		p.Cmd.Process.Kill()
		<-p.exited
		return fmt.Errorf("the app did not exit within %v after quit", timeout)
	}
}

// WaitTokenFile waits up to timeout for the token file at path, which a
// program with the automation channel writes once the channel is open; a
// close of exited ends the wait early.
func WaitTokenFile(path string, exited <-chan struct{}, timeout time.Duration) (wire.TokenFile, error) {
	return waitTokenFile(context.Background(), path, exited, nil, timeout)
}

// waitTokenFile is WaitTokenFile naming the program's exit error, when
// exitErr is set, in the message, and giving up when ctx is done.
func waitTokenFile(ctx context.Context, path string, exited <-chan struct{}, exitErr func() error, timeout time.Duration) (wire.TokenFile, error) {
	deadline := time.Now().Add(timeout)
	for {
		tf, err := wire.ReadTokenFile(path)
		if err == nil {
			return tf, nil
		}
		select {
		case <-exited:
			status := ""
			if exitErr != nil {
				status = fmt.Sprintf(" (%v)", exitErr())
			}
			return tf, fmt.Errorf("the program exited before opening its control channel%s; is it a Tango app built with -tags tango_dev?", status)
		default:
		}
		if time.Now().After(deadline) {
			return tf, fmt.Errorf("no control channel after %v (does the program create a tango.App, and was it built with -tags tango_dev?)", timeout)
		}
		select {
		case <-ctx.Done():
			return tf, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Build returns an executable for target: target itself (as an absolute
// path, resolved against dir when relative) when it is an executable
// with the automation channel, an error when it is one without, else the
// package target built with `go build -tags tango_dev` (plus buildFlags)
// in dir into outDir/app (app.exe on Windows).
func Build(target, dir string, buildFlags []string, outDir string) (string, error) {
	return build(context.Background(), target, dir, buildFlags, outDir)
}

// build is Build, stopping go build when ctx is done.
func build(ctx context.Context, target, dir string, buildFlags []string, outDir string) (string, error) {
	p := target
	if dir != "" && !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
		if !HasChannel(p) {
			return "", fmt.Errorf("%s has no automation channel: build it with -tags tango_dev (production builds, the default, leave the channel out)", p)
		}
		return filepath.Abs(p)
	}
	bin := filepath.Join(outDir, "app")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	goTool := "go"
	if p, err := exec.LookPath("go"); err == nil {
		goTool = p
	} else if r := runtime.GOROOT(); r != "" {
		goTool = filepath.Join(r, "bin", "go")
	}
	args := append([]string{"build", "-o", bin}, WithDevTag(buildFlags)...)
	args = append(args, target)
	cmd := exec.CommandContext(ctx, goTool, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("go build %s: %v\n%s", target, err, out)
	}
	return bin, nil
}

// WithDevTag returns build flags with tango_dev added to their -tags (the
// last -tags flag wins in go build, so it is merged into that one).
func WithDevTag(flags []string) []string {
	out := slices.Clone(flags)
	last := -1 // index of the last -tags value
	for i := 0; i < len(out); i++ {
		f := out[i]
		switch {
		case f == "-tags" || f == "--tags":
			if i+1 < len(out) {
				last = i + 1
				i++
			}
		case strings.HasPrefix(f, "-tags=") || strings.HasPrefix(f, "--tags="):
			last = i
		}
	}
	if last < 0 {
		return append([]string{"-tags", devTag}, out...)
	}
	prefix, v := "", out[last]
	if k, val, ok := strings.Cut(v, "="); ok && strings.HasPrefix(k, "-") {
		prefix, v = k+"=", val
	}
	tags := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	if !slices.Contains(tags, devTag) {
		tags = append(tags, devTag)
	}
	out[last] = prefix + strings.Join(tags, ",")
	return out
}

// HasChannel reports whether the executable at path links the automation
// channel (it names the channel's environment variable); a file that
// cannot be read is given the benefit of the doubt.
func HasChannel(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	return bytes.Contains(b, []byte(wire.EnvTokenFile))
}

// LaunchEnv is the environment of a launched program. It starts in the
// background (macOS: no activation, its windows behind the others) so a
// launch does not take focus from the user or cover windows other tools
// measure; environ and extra come after, so TANGO_LAUNCH_BACKGROUND=0
// there restores the activation (exec keeps the last value of a key).
// The channel address and token file always win.
func LaunchEnv(environ, extra []string, addr, tokenFile string) []string {
	env := append([]string{EnvLaunchBackground + "=1"}, filterEnv(environ)...)
	env = append(env, extra...)
	return append(env, wire.EnvAddr+"="+addr, wire.EnvTokenFile+"="+tokenFile)
}

func filterEnv(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, wire.EnvAddr+"=") || strings.HasPrefix(kv, wire.EnvTokenFile+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
