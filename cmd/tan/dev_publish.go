package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// publishAndStart prepares a complete candidate on the stable output's
// volume. Only publication and startup need the cross-process lock.
func (s *devSession) publishAndStart(ctx context.Context, binary string, consumeBinary bool, command func() *exec.Cmd, stopCurrent func()) (*process, error) {
	ctx = s.startupContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir := devDir(s.out)
	stage, err := os.MkdirTemp(dir, ".publish-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(stage)
	output := filepath.Join(stage, filepath.Base(s.out))
	src, dst := output, s.out
	if runtime.GOOS == "darwin" {
		dst = s.app()
		src = filepath.Join(stage, filepath.Base(dst))
		output = filepath.Join(src, "Contents", "MacOS", filepath.Base(s.out))
	}
	if binary != "" {
		stageBinary := installBinary
		if runtime.GOOS == "darwin" && consumeBinary {
			stageBinary = movePrivateDevBinary
		}
		if err := stageBinary(ctx, binary, output); err != nil {
			return nil, fmt.Errorf("staging the app: %w", err)
		}
	} else {
		if err := s.seedDevOutput(ctx, output, false); err != nil {
			return nil, err
		}
		logf("Building")
		if err := s.goBuild(ctx, output, false, os.Stderr); err != nil {
			return nil, fmt.Errorf("building the app: %w", err)
		}
		if _, err := os.Stat(output); err != nil {
			return nil, fmt.Errorf("checking build output: %w", err)
		}
	}
	if runtime.GOOS == "darwin" {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := writeBundle(s.cfg, src); err != nil {
			return nil, fmt.Errorf("writing the app bundle: %w", err)
		}
		if !lookTool("codesign") {
			return nil, errors.New("codesign is required to publish a macOS dev app")
		}
		if err := adhocSign(ctx, src); err != nil {
			return nil, err
		}
	}

	if err := s.prepareLaunch(); err != nil {
		return nil, err
	}
	unlock, err := lockDevPublication(ctx, dir)
	if err != nil {
		return nil, fmt.Errorf("locking the app output: %w", err)
	}
	defer unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" && stopCurrent != nil {
		// An owned process can hold the executable open. A different
		// session's process remains untouched if replacement fails.
		stopCurrent()
	}
	if err := s.checkLaunch(); err != nil {
		return nil, err
	}
	if err := publishDevOutput(ctx, src, dst); err != nil {
		return nil, fmt.Errorf("publishing the app: %w", err)
	}
	if runtime.GOOS != "windows" && stopCurrent != nil {
		stopCurrent()
	}
	if err := s.checkLaunch(); err != nil {
		return nil, err
	}
	return startProcess(command())
}

// movePrivateDevBinary consumes the session's initial probe. It is already
// a private executable, so signing the staged bundle can use the same inode.
func movePrivateDevBinary(ctx context.Context, src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	err := os.Rename(src, dst)
	if errors.Is(err, syscall.EXDEV) {
		return installBinary(ctx, src, dst)
	}
	return err
}

// mayDryRun conservatively skips seeding for any explicit -n setting. A
// fresh target lets us distinguish Go's dry-run success from a real build.
func (s *devSession) mayDryRun(ctx context.Context) bool {
	flags := os.Getenv("GOFLAGS")
	if flags == "" {
		var out strings.Builder
		if s.startup != nil {
			cmd := exec.Command("go", "env", "GOFLAGS")
			cmd.Dir, cmd.Stdout = s.cfg.dir, &out
			if err := runStartupCommand(ctx, cmd); err != nil {
				return true
			}
		} else {
			cmd := exec.CommandContext(ctx, "go", "env", "GOFLAGS")
			cmd.Dir, cmd.Stdout = s.cfg.dir, &out
			if err := cmd.Run(); err != nil {
				return true
			}
		}
		flags = out.String()
	}
	// Go accepts adjacent quoted and unquoted tokens (e.g. '"-x"-n').
	// A substring match also skips some benign flags, which only costs a link.
	return strings.Contains(flags, "-n")
}

// lockDevPublication uses a stable lock file outside the replaceable app.
// Leave the file in place so all contenders lock the same inode.
func lockDevPublication(ctx context.Context, dir string) (func(), error) {
	f, err := os.OpenFile(filepath.Join(dir, ".publish.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for ctx.Err() == nil {
		unlock, err := tryDevPublicationLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if unlock != nil {
			return func() { unlock(); f.Close() }, nil
		}
		select {
		case <-ctx.Done():
		case <-tick.C:
		}
	}
	f.Close()
	return nil, ctx.Err()
}
