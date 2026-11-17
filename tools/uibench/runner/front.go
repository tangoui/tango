package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Ruling R51: occluded windows. MyGo pauses its animation frames while its
// window is occluded (mygo@v0.9.0 ui/runtime.go: `case moving &&
// rt.host.occluded(): rt.held = true`); Tango and NSGo keep drawing. With
// another app frontmost on the desktop, MyGo drew 0 frames after ready and
// the comparison was invalid. So every attempt re-raises its window just
// before sampling and verifies it is frontmost, and an attempt that drew
// far below the expected rate is disturbed and measured again.

const (
	expectedFPS    = 60.0
	minDrawShare   = 0.5 // an attempt below this share of expectedFPS is disturbed
	frontRetries   = 3   // raises after the first when another app stays frontmost
	frontRetryWait = 300 * time.Millisecond
	noteNotDrawing = "occluded or not drawing"
)

// frontProbe raises a window and reads which process is frontmost:
// bringToFront and frontmostPID in production, fakes in the tests.
type frontProbe struct {
	raise func(pid int) error
	front func() (int, error) // the frontmost process' unix id
	sleep func(time.Duration)
}

// raiseVerified raises pid's window and checks that pid is the frontmost
// process, raising again up to frontRetries times if it is not. notes say
// what went wrong on the way; verified is false when pid never was
// frontmost (or the check could not be made).
func raiseVerified(p frontProbe, pid int) (verified bool, notes []string) {
	var last string
	for try := 0; try <= frontRetries; try++ {
		if try > 0 && p.sleep != nil {
			p.sleep(frontRetryWait)
		}
		if err := p.raise(pid); err != nil {
			last = "raise: " + err.Error()
			continue
		}
		got, err := p.front()
		switch {
		case err != nil:
			last = "frontmost check: " + err.Error()
		case got != pid:
			last = fmt.Sprintf("frontmost pid %d, not the scene's %d", got, pid)
		default:
			if try > 0 {
				notes = append(notes, fmt.Sprintf("frontmost after %d raises", try+1))
			}
			return true, notes
		}
	}
	return false, append(notes, fmt.Sprintf("frontmost not verified after %d raises: %s", frontRetries+1, last))
}

// drawCheckFPS is the frame rate an attempt of scene must reach, and
// whether the check applies at all: not to counter (no frame is requested
// after ready, expected fps 0) nor to a sparse scene whose frame rate the
// protocol defines (sceneMeta.sparseFrames).
func drawCheckFPS(scene string) (float64, bool) {
	if scene == "counter" || metaOf(scene).sparseFrames {
		return 0, false
	}
	return minDrawShare * expectedFPS, true
}

// checkDrawing marks a ready, error-free attempt disturbed when it drew
// below drawCheckFPS: its window was occluded (or it stopped drawing), so
// it measured a different state. runSceneRound then measures it again.
func checkDrawing(r *Round) {
	if !r.Ready || r.Error != "" {
		return
	}
	min, ok := drawCheckFPS(r.Scene)
	if !ok || r.FPS >= min {
		return
	}
	r.Disturbed = true
	if !contains(r.Notes, noteNotDrawing) {
		r.Notes = append(r.Notes, noteNotDrawing)
	}
}

// scenesFromInvalid reads a previous results file and returns, in its
// order, every scene whose verdict was Invalid against any opponent
// (-scenes-from-invalid).
func scenesFromInvalid(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var res Results
	if err := json.Unmarshal(b, &res); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []string
	for _, v := range res.Verdicts {
		if v.Verdict == Invalid && !contains(out, v.Scene) {
			out = append(out, v.Scene)
		}
	}
	if len(res.Verdicts) == 0 {
		return nil, fmt.Errorf("%s: no verdicts (an unfinished run, or one without tango)", path)
	}
	return out, nil
}

// profileScenes is -profile-dir: each scene once per framework, sampled as
// usual (load gate, no idle samples, no retries), the Tango program writing
// its CPU profile (sceneCommand). It reports each Tango profile it finds.
func profileScenes(d roundDeps, scenes, fws []string, dir string, emit func(Round) error) {
	for _, scene := range scenes {
		for _, fw := range fws {
			if d.gate != nil {
				d.gate()
			}
			r := d.measure(fw, scene)
			checkDrawing(&r)
			if d.processOnly {
				r.CPUMetric = metricProcess
			}
			if fw == "tango" {
				p := profilePath(dir, scene)
				if st, err := os.Stat(p); err != nil || st.Size() == 0 {
					r.Notes = append(r.Notes, "no cpu profile at "+p)
				} else {
					r.Notes = append(r.Notes, "cpu profile "+p)
				}
			}
			if err := emit(r); err != nil {
				fatal(err)
			}
		}
	}
}
