package main

import (
	"context"
	"os"
)

// iosSimDev deploys to an iOS Simulator through xcrun simctl.
type iosSimDev struct {
	cfg     *Config
	run     devRunner
	buildFn func(ctx context.Context) (string, error)
	pick    func(ctx context.Context) (simDevice, error) // run.go's simDevices + pickSimulator

	udid, name string
	app        bgProcess // simctl launch --console-pty: the app's output
}

func (s *iosSimDev) describe() string { return "the " + s.name + " Simulator" }

func (s *iosSimDev) prepare(ctx context.Context, url string) error {
	d, err := s.pick(ctx)
	if err != nil {
		return err
	}
	s.udid, s.name = d.UDID, d.Name
	if d.State != "Booted" {
		logf("Booting the %s Simulator", d.Name)
		// bootstatus -b boots the device and waits until it is up.
		if err := s.run.run(ctx, "xcrun", "simctl", "bootstatus", s.udid, "-b"); err != nil {
			return err
		}
		_ = s.run.run(ctx, "open", "-a", "Simulator", "--args", "-CurrentDeviceUDID", s.udid)
	}
	// The Simulator shares this machine's network: localhost URLs work as is.
	return nil
}

func (s *iosSimDev) build(ctx context.Context) (string, error) { return s.buildFn(ctx) }

func (s *iosSimDev) install(ctx context.Context, app string) error {
	return s.run.run(ctx, "xcrun", "simctl", "install", s.udid, app)
}

func (s *iosSimDev) launch(ctx context.Context, url string) error {
	s.stop(ctx)
	var env []string
	if url != "" {
		// simctl passes SIMCTL_CHILD_X to the app as X.
		env = append(env, "SIMCTL_CHILD_TANGO_DEV_URL="+url)
	}
	// --console-pty connects the app's stdout and stderr (Go's log) to
	// this terminal until it exits.
	p, err := s.run.start(env, os.Stdout, "xcrun", "simctl", "launch", "--console-pty", s.udid, s.cfg.ID)
	if err != nil {
		return err
	}
	s.app = p
	return nil
}

func (s *iosSimDev) exited() <-chan struct{} {
	if s.app == nil {
		return nil
	}
	return s.app.done()
}

func (s *iosSimDev) stop(ctx context.Context) {
	if s.udid == "" {
		return
	}
	_ = s.run.run(ctx, "xcrun", "simctl", "terminate", s.udid, s.cfg.ID)
	if s.app != nil {
		s.app.stop()
		s.app = nil
	}
}

// cleanup leaves the Simulator running for the next session.
func (s *iosSimDev) cleanup(ctx context.Context) {}
