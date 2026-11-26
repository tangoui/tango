// Package skilldocs checks the Go examples published with the Tango app skill.
package skilldocs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Documentation fences contain declarations for this shared package. The
// compiler checks argument and return types against the checkout, including
// calls to methods whose names alone can be checked with go doc.
func TestGoSnippetsCompile(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	docs, err := filepath.Glob(filepath.Join(root, "skills", "tango", "references", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	fence := regexp.MustCompile("(?ms)^```go\\r?\\n(.*?)^```[ \\t]*\\r?$")
	var code strings.Builder
	code.WriteString(`package snippets
import (
	"context"
	"errors"
	"os"
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/service/paths"
	"github.com/tangoui/tango/sys/dialog"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/notify"
	"github.com/tangoui/tango/sys/tray"
	"github.com/tangoui/tango/sys/windowstate"
	"github.com/tangoui/tango/ui"
)
`)
	count := 0
	for _, doc := range docs {
		data, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, block := range fence.FindAllSubmatchIndex(data, -1) {
			line := strings.Count(string(data[:block[2]]), "\n") + 1
			fmt.Fprintf(&code, "//line %s:%d\n", doc, line)
			code.Write(data[block[2]:block[3]])
			count++
		}
	}
	if count == 0 {
		t.Fatal("no Go snippets found")
	}
	src := filepath.Join(t.TempDir(), "snippets.go")
	if err := os.WriteFile(src, []byte(code.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", src)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile %d skill snippets: %v\n%s", count, err, output)
	}
}
