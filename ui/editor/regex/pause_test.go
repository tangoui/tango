package regex

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dlclark/regexp2"
)

func TestTimeoutAfterPauseIsConfirmed(t *testing.T) {
	for _, input := range []string{"xy", "1"} {
		t.Run(input, func(t *testing.T) {
			re := mustCompile(t, "x(?=y)", "").(*backtrackRegexp)
			now := time.Now()
			calls := 0
			find := func(rs []rune) (*regexp2.Match, error) {
				calls++
				if calls == 1 {
					now = now.Add(10 * time.Second) // paused after regexp2 created its deadline
					return nil, errors.New("match timeout after simulated pause")
				}
				return re.re.FindRunesMatch(rs)
			}
			_, _, before := Stats()
			got, err := re.tryMatchStart(input, nil, find, func() time.Time { return now })
			want := re.MatchStart(input)
			if err != nil || !reflect.DeepEqual(got, want) || calls != 2 {
				t.Fatalf("got %v, %v (%d attempts); want %v after confirmation", got, err, calls, want)
			}
			if _, _, after := Stats(); after != before {
				t.Fatal("unconfirmed pause counted as timeout")
			}
		})
	}
}

func TestOriginal100msAttemptIsPreserved(t *testing.T) {
	re := mustCompile(t, `(?:(a+)+(?=b)|a+c)`, "").(*backtrackRegexp)
	// Configuration, not a speed assumption: each attempt must retain the
	// original engine's entire allowance, including the confirmation attempt.
	if re.re.MatchTimeout != matchTimeout {
		t.Fatalf("engine allowance=%v, want %v", re.re.MatchTimeout, matchTimeout)
	}
	original, err := regexp2.Compile(re.re.String(), regexp2.ECMAScript)
	if err != nil {
		t.Fatal(err)
	}
	original.MatchTimeout = matchTimeout
	input := strings.Repeat("a", 20) + "c"
	baseline, err := original.FindStringMatch(input)
	if err != nil {
		t.Skipf("original 100ms engine timed out under current load: %v", err)
	}
	if baseline == nil {
		t.Fatal("original engine unexpectedly did not match")
	}
	got, err := re.TryMatchStart(input, nil)
	if err != nil || len(got) < 2 || got[0] != baseline.Index || got[1] != baseline.Index+baseline.Length {
		t.Fatalf("wrapper=%v, %v; original match=%v", got, err, baseline)
	}
}

func TestRepeatedTimeoutIsCountedOnce(t *testing.T) {
	re := mustCompile(t, "x(?=y)", "").(*backtrackRegexp)
	now := time.Now()
	calls := 0
	find := func([]rune) (*regexp2.Match, error) {
		calls++
		now = now.Add(matchTimeout)
		return nil, errors.New("match timeout")
	}
	_, _, before := Stats()
	got, err := re.tryMatchStart("xy", nil, find, func() time.Time { return now })
	if got != nil || !errors.Is(err, ErrTimeout) || calls != 2 {
		t.Fatalf("got %v %v after %d attempts", got, err, calls)
	}
	if _, _, after := Stats(); after != before+1 {
		t.Fatal("confirmed timeout must be counted once")
	}
}
