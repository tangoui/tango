// Command drillapp is the app of the macOS update drill (see
// ../drill_test.go): a headless Tango app whose version is set at link
// time. It reads drill.json next to its bundle, logs what happens, and on
// each launch confirms the previous update, then looks for a newer
// release and installs it.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/service/updater"
)

var (
	version = "0.0.0"
	// mode "noconfirm" plays a broken release that never calls Confirm.
	mode = ""
)

// Linked so the executable has the size and shape of a real Tango app.
var _ = tango.New

type config struct {
	ManifestURL string `json:"manifest_url"`
	PublicKey   string `json:"public_key"`
	StateDir    string `json:"state_dir"`
	Log         string `json:"log"`
	Timeout     int    `json:"timeout"`
}

func main() {
	exe, _ := os.Executable()
	exe, _ = filepath.EvalSymlinks(exe)
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	var c config
	b, err := os.ReadFile(filepath.Join(filepath.Dir(bundle), "drill.json"))
	if err == nil {
		err = json.Unmarshal(b, &c)
	}
	if err != nil {
		os.Exit(3)
	}
	logf := func(format string, a ...any) {
		f, err := os.OpenFile(c.Log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return
		}
		fmt.Fprintf(f, "%s pid=%d "+format+"\n", append([]any{version, os.Getpid()}, a...)...)
		f.Close()
	}
	extra(logf)
	if mode == "noconfirm" {
		logf("started (broken: not confirming)")
		return
	}
	u := &updater.Updater{
		ManifestURL:    c.ManifestURL,
		PublicKey:      c.PublicKey,
		Version:        version,
		AppID:          "dev.tango.drill",
		StateDir:       c.StateDir,
		ConfirmTimeout: time.Duration(c.Timeout) * time.Second,
	}
	o, err := u.Confirm()
	if err != nil {
		logf("confirm error: %v", err)
		return
	}
	logf("started outcome=%s from=%s reason=%q", o.Status, o.From, o.Reason)
	if o.Status != updater.StatusNone {
		return // one step per launch keeps the drill deterministic
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	rel, err := u.Check(ctx)
	if err != nil {
		logf("check error: %v", err)
		return
	}
	if rel == nil {
		logf("up to date")
		return
	}
	p, err := u.Download(ctx, rel, "")
	if err != nil {
		logf("download error: %v", err)
		return
	}
	logf("downloaded %s patched=%v", rel.Version, p.Patched)
	if err := u.Apply(p); err != nil {
		logf("apply error: %v", err)
		return
	}
	logf("applied; quitting")
}
