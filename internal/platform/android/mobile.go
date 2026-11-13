package android

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

// Pure logic of the mobile capabilities (mobile_android.go): decoding what
// the Java side (org.tango.host.Mobile) answers. Java reports failures as
// "!code message"; the codes map to the platform errors.

// javaErr turns a Java error string into an error; nil for "".
func javaErr(s string) error {
	if s == "" {
		return nil
	}
	code, msg, _ := strings.Cut(strings.TrimPrefix(s, "!"), " ")
	var base error
	switch code {
	case "canceled":
		base = platform.ErrCanceled
	case "unsupported":
		base = platform.ErrUnsupported
	case "notAvailable":
		base = platform.ErrNotAvailable
	case "notEnrolled":
		base = platform.ErrNotEnrolled
	case "lockedOut":
		base = platform.ErrLockedOut
	case "authFailed":
		base = platform.ErrAuthFailed
	case "permissionDenied":
		base = platform.ErrPermissionDenied
	case "notFound":
		base = platform.ErrNotFound
	case "notConfigured":
		base = platform.ErrNotConfigured
	default:
		return errors.New("tango/android: " + strings.TrimPrefix(s, "!"))
	}
	if msg == "" {
		return base
	}
	return &codedErr{base, msg}
}

type codedErr struct {
	base error
	msg  string
}

func (e *codedErr) Error() string { return e.base.Error() + ": " + e.msg }
func (e *codedErr) Unwrap() error { return e.base }

// splitResult separates a synchronous Java result from its error.
func splitResult(s string) (string, error) {
	if strings.HasPrefix(s, "!") {
		return "", javaErr(s)
	}
	return s, nil
}

// javaLocation is a fix as Mobile.java encodes it.
type javaLocation struct {
	Lat, Lon, Alt, Acc, AltAcc, Speed, Heading float64
	Time                                       int64 // ms since the epoch
}

func decodeLocation(s string) (platform.Location, error) {
	var j javaLocation
	if err := json.Unmarshal([]byte(s), &j); err != nil {
		return platform.Location{}, errors.New("tango/android: bad location " + s)
	}
	return platform.Location{
		Latitude: j.Lat, Longitude: j.Lon, Altitude: j.Alt,
		Accuracy: j.Acc, AltitudeAccuracy: j.AltAcc, Speed: j.Speed, Heading: j.Heading,
		Time: time.UnixMilli(j.Time),
	}, nil
}

func decodePaths(s string) ([]string, error) {
	var ps []string
	if err := json.Unmarshal([]byte(s), &ps); err != nil {
		return nil, errors.New("tango/android: bad path list " + s)
	}
	return ps, nil
}

// args encodes call arguments for Mobile.call.
func args(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// eventQueue holds app events reported before the backend delivered
// Started; they are delivered right after it. Main thread only.
type eventQueue struct {
	pending []platform.AppEvent
}

func (q *eventQueue) add(ev platform.AppEvent) { q.pending = append(q.pending, ev) }

func (q *eventQueue) take() []platform.AppEvent {
	p := q.pending
	q.pending = nil
	return p
}

// javaTask is a background job update as Device.java encodes it.
type javaTask struct {
	ID       int64
	Kind     int
	Deadline int64 // ms since the epoch; 0 unknown
	Expired  bool
}

func decodeBackground(s string) []platform.BackgroundTask {
	var js []javaTask
	if json.Unmarshal([]byte(s), &js) != nil {
		return nil
	}
	out := make([]platform.BackgroundTask, 0, len(js))
	for _, j := range js {
		t := platform.BackgroundTask{ID: j.ID, Kind: platform.BackgroundKind(j.Kind), Expired: j.Expired}
		if j.Deadline > 0 {
			t.Deadline = time.UnixMilli(j.Deadline)
		}
		out = append(out, t)
	}
	return out
}
