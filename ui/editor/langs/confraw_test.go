package langs

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// ConfRaw keeps the full original language configuration, including fields
// Conf does not model (onEnterRules, autoCloseBefore, colorizedBracketPairs...).
func TestConfRaw(t *testing.T) {
	for _, id := range []string{"go", "typescript"} {
		l := ByID(id)
		raw, err := os.ReadFile("data/" + id + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var f struct {
			Conf json.RawMessage `json:"conf"`
		}
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatal(err)
		}
		var want, got any
		if err := json.Unmarshal(f.Conf, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(l.ConfRaw, &got); err != nil {
			t.Fatalf("%s: ConfRaw is not JSON: %v", id, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: ConfRaw differs from the exported conf", id)
		}
	}
	if !bytes.Contains(ByID("go").ConfRaw, []byte(`"surroundingPairs"`)) {
		t.Error(`go ConfRaw lacks "surroundingPairs"`)
	}
	if !bytes.Contains(ByID("typescript").ConfRaw, []byte(`"onEnterRules"`)) {
		t.Error(`typescript ConfRaw lacks "onEnterRules"`)
	}
	for _, l := range Builtin() {
		if len(l.ConfRaw) == 0 {
			t.Errorf("%s: empty ConfRaw", l.ID)
		}
	}
}

// NOTICE must describe what the exporter changes rather than claim the data
// is unmodified.
func TestNoticeDescribesExporterChanges(t *testing.T) {
	b, err := os.ReadFile("NOTICE")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "otherwise unmodified") {
		t.Error(`NOTICE still says "otherwise unmodified"`)
	}
	for _, want := range []string{"registrationOrder", "normalized", "vue.json", "byte-identical", "sql.json"} {
		if !strings.Contains(s, want) {
			t.Errorf("NOTICE does not mention %q", want)
		}
	}
}
