package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// helperJob is what the helper process does after the app exits. On macOS
// and Linux it runs as a /bin/sh script (helperScript); on Windows the
// app starts a copy of its own executable, which runs runJob from this
// package's init (helper_windows.go).
type helperJob struct {
	PID           int      `json:"pid"`    // the app; wait for it to exit
	Format        string   `json:"format"` // Asset.Format
	Target        string   `json:"target"` // the installation
	Repl          string   `json:"repl"`   // its replacement, or the installer
	Work          string   `json:"work"`   // next to Target; receives the previous version
	State         string   `json:"state"`  // Updater.StateDir
	Timeout       int      `json:"timeout"`
	Launch        []string `json:"launch"` // starts the installed app
	InstallerArgs []string `json:"installer_args,omitempty"`
}

// helperOps are runJob's dealings with processes, replaced in tests.
type helperOps struct {
	wait  func(pid int)            // returns once pid has exited
	start func(cmd []string) error // starts cmd without waiting
	run   func(cmd []string) error // runs cmd to completion
	alive func(pid int) bool       // reports whether pid runs
	sleep func(d time.Duration)
}

// runJob swaps the installation, starts it and waits for the new version
// to confirm, rolling back otherwise. It mirrors helperScript.
func runJob(j *helperJob, ops helperOps) error {
	ops.wait(j.PID)
	prev := filepath.Join(j.Work, "previous")
	rolledBack := func(reason string) {
		if j.State != "" {
			os.WriteFile(filepath.Join(j.State, stateRolledBack), []byte(reason+"\n"), 0o600)
		}
	}

	var installErr error
	switch j.Format {
	case FormatMSI:
		// Windows Installer replaces files transactionally and keeps no
		// copy the helper could restore: install, start, done.
		name, args, err := installerCommand(j.Repl, filepath.Dir(j.Target), j.InstallerArgs)
		if err == nil {
			err = ops.run(append([]string{name}, args...))
		}
		if err != nil {
			rolledBack("installer failed: " + err.Error())
		} else {
			os.Remove(filepath.Join(j.State, statePending))
		}
		removeAllRetry(j.Work, ops)
		return ops.start(j.Launch)
	case FormatNSIS:
		// Keep the old executable aside; the installer writes the new one.
		if err := os.Rename(j.Target, prev); err != nil {
			installErr = err
			break
		}
		name, args, err := installerCommand(j.Repl, filepath.Dir(j.Target), j.InstallerArgs)
		if err == nil {
			err = ops.run(append([]string{name}, args...))
		}
		if err == nil {
			if _, serr := os.Stat(j.Target); serr != nil {
				err = errors.New("the installer did not install " + filepath.Base(j.Target))
			}
		}
		if err != nil {
			installErr = err
			os.Remove(j.Target)
			os.Rename(prev, j.Target)
		}
	default:
		if err := os.Rename(j.Target, prev); err != nil {
			installErr = err
			break
		}
		if err := os.Rename(j.Repl, j.Target); err != nil {
			installErr = err
			os.Rename(prev, j.Target)
		}
	}
	if installErr != nil {
		rolledBack("install failed: " + installErr.Error())
		removeAllRetry(j.Work, ops)
		return ops.start(j.Launch)
	}

	if err := ops.start(j.Launch); err != nil || j.State == "" {
		if err != nil {
			return restore(j, ops, prev, "could not start the new version: "+err.Error())
		}
		removeAllRetry(j.Work, ops)
		return nil
	}
	confirmed := filepath.Join(j.State, stateConfirmed)
	request := filepath.Join(j.State, stateRollback)
	deadline := time.Duration(j.Timeout) * time.Second
	for waited := time.Duration(0); ; waited += 100 * time.Millisecond {
		if _, err := os.Stat(confirmed); err == nil {
			removeAllRetry(j.Work, ops)
			os.Remove(filepath.Join(j.State, statePending))
			os.Remove(confirmed)
			return nil
		}
		if b, err := os.ReadFile(request); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				for ops.alive(pid) {
					ops.sleep(100 * time.Millisecond)
				}
			}
			os.Remove(request)
			return restore(j, ops, prev, "the new version asked for a rollback")
		}
		if waited >= deadline {
			return restore(j, ops, prev, fmt.Sprintf("the new version did not confirm within %ds", j.Timeout))
		}
		ops.sleep(100 * time.Millisecond)
	}
}

// restore puts the previous version back and starts it.
func restore(j *helperJob, ops helperOps, prev, reason string) error {
	failed := filepath.Join(j.Work, "failed")
	if err := os.Rename(j.Target, failed); err != nil && !errors.Is(err, os.ErrNotExist) {
		reason += "; could not move the new version aside: " + err.Error()
	}
	if err := os.Rename(prev, j.Target); err != nil {
		// Leave the work directory: it holds the previous version.
		os.WriteFile(filepath.Join(j.State, stateRolledBack), []byte(reason+"; restoring failed: "+err.Error()+"\n"), 0o600)
		return err
	}
	os.WriteFile(filepath.Join(j.State, stateRolledBack), []byte(reason+"\n"), 0o600)
	removeAllRetry(j.Work, ops)
	return ops.start(j.Launch)
}

// removeAllRetry removes dir, retrying for a while: on Windows a file
// stays locked until the process that ran it has fully exited.
func removeAllRetry(dir string, ops helperOps) {
	for range 50 {
		if os.RemoveAll(dir) == nil {
			return
		}
		ops.sleep(200 * time.Millisecond)
	}
}

// helperScript runs in /bin/sh after Apply on macOS and Linux; it does
// what runJob does for the replacing formats:
//
//	$1 pid to wait for   $2 target   $3 replacement   $4 work dir
//	$5 state dir ("" for no confirmation)   $6 confirm timeout (seconds)
//	$7… command that starts the app
//
// Once the process has exited it renames target into the work directory
// and the replacement onto target (renaming the old target back if that
// fails), starts the app, and waits for the state dir's "confirmed" file:
// then it removes the work directory with the previous version. On
// timeout, or when "rollback" appears (after the pid in it has exited),
// it moves the new version aside, restores the previous one, writes the
// reason to "rolledback" and starts the app again.
const helperScript = `pid=$1 target=$2 repl=$3 work=$4 state=$5 timeout=$6
shift 6
trap '' HUP INT
while kill -0 "$pid" 2>/dev/null; do sleep 0.1; done
note() { [ -n "$state" ] && printf '%s\n' "$1" > "$state/rolledback"; }
start() { if [ $# -gt 0 ]; then "$@" </dev/null >/dev/null 2>&1 & fi; }
swapped=
if mv -f "$target" "$work/previous"; then
	if mv -f "$repl" "$target"; then swapped=1; else mv -f "$work/previous" "$target"; fi
fi
if [ -z "$swapped" ]; then
	note "install failed: could not replace $target"
	rm -rf "$work"; start "$@"; exit 0
fi
start "$@"
if [ -z "$state" ]; then rm -rf "$work"; exit 0; fi
n=0 limit=$((timeout * 10))
while :; do
	if [ -e "$state/confirmed" ]; then
		rm -rf "$work"; rm -f "$state/pending.json" "$state/confirmed"; exit 0
	fi
	if [ -e "$state/rollback" ]; then
		rpid=$(cat "$state/rollback")
		while [ -n "$rpid" ] && kill -0 "$rpid" 2>/dev/null; do sleep 0.1; done
		rm -f "$state/rollback"; reason="the new version asked for a rollback"; break
	fi
	if [ $n -ge $limit ]; then reason="the new version did not confirm within ${timeout}s"; break; fi
	sleep 0.1; n=$((n + 1))
done
mv -f "$target" "$work/failed"
if mv -f "$work/previous" "$target"; then
	note "$reason"; rm -rf "$work"
else
	note "$reason; restoring failed"
fi
start "$@"
`

// helperArgs returns the arguments of /bin/sh for helperScript.
func helperArgs(j *helperJob) []string {
	return append([]string{"-c", helperScript, "tango-update", strconv.Itoa(j.PID), j.Target, j.Repl, j.Work, j.State, strconv.Itoa(j.Timeout)}, j.Launch...)
}
