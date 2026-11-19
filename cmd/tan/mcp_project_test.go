package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpClient serves s over in-memory transports and returns a connected
// client; both end with the test.
func mcpClient(t *testing.T, s *mcpServer) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := s.srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

// projectServer returns a server on a fresh root and a client for it.
func projectServer(t *testing.T) (*mcpServer, *mcp.ClientSession) {
	t.Helper()
	s, err := newMCPServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.close)
	return s, mcpClient(t, s)
}

// call calls a tool; it fails the test on a protocol error.
func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	if args == nil {
		args = map[string]any{}
	}
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

// callOK calls a tool that must succeed and decodes its JSON result into out.
func callOK(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	res := call(t, cs, name, args)
	if res.IsError {
		t.Fatalf("%s(%v): tool error: %s", name, args, resultText(res))
	}
	if out != nil {
		if err := json.Unmarshal([]byte(resultText(res)), out); err != nil {
			t.Fatalf("%s: decoding %q: %v", name, resultText(res), err)
		}
	}
}

// callErr calls a tool that must fail and returns its error text.
func callErr(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res := call(t, cs, name, args)
	if !res.IsError {
		t.Fatalf("%s(%v) succeeded, want a tool error: %s", name, args, resultText(res))
	}
	return resultText(res)
}

// runTestBinaryAsTan makes jobs and tan init run this test binary as tan
// (TestMain), with the network off for go mod tidy.
func runTestBinaryAsTan(t *testing.T) {
	t.Helper()
	oldExe, oldEnv := selfExe, selfEnv
	selfExe = func() (string, error) { return os.Executable() }
	selfEnv = []string{"TAN_TEST_MAIN=1", "GOPROXY=off", "GOFLAGS=-mod=mod"}
	t.Cleanup(func() { selfExe, selfEnv = oldExe, oldEnv })
}

func TestProjectInspectVanilla(t *testing.T) {
	runTestBinaryAsTan(t)
	s, cs := projectServer(t)
	callOK(t, cs, "tango_project_init", map[string]any{"dir": "app", "template": "vanilla", "name": "Hello", "module": "example.com/hello"}, nil)
	var got struct {
		Dir    string         `json:"dir"`
		Config map[string]any `json:"config"`
		Module string         `json:"module"`
		Kind   string         `json:"kind"`
	}
	callOK(t, cs, "tango_project_inspect", map[string]any{"dir": "app"}, &got)
	if got.Kind != "webview" {
		t.Errorf("kind = %q, want webview", got.Kind)
	}
	if got.Config["name"] != "Hello" {
		t.Errorf("config.name = %v, want Hello", got.Config["name"])
	}
	if got.Module != "example.com/hello" {
		t.Errorf("module = %q, want example.com/hello", got.Module)
	}
	if want := filepath.Join(s.root, "app"); got.Dir != want {
		t.Errorf("dir = %q, want %q", got.Dir, want)
	}
}

func TestProjectInspectKinds(t *testing.T) {
	s, cs := projectServer(t)
	if err := os.MkdirAll(filepath.Join(s.root, "ui"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"name": "Draw", "id": "com.example.draw", "version": "0.1.0"}`
	if err := os.WriteFile(filepath.Join(s.root, "ui", ConfigFile), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Kind             string `json:"kind"`
		Error            string `json:"error"`
		HasFrontendBuild bool   `json:"hasFrontendBuild"`
	}
	callOK(t, cs, "tango_project_inspect", map[string]any{"dir": "ui"}, &got)
	if got.Kind != "ui" || got.HasFrontendBuild {
		t.Errorf("self-drawn project: %+v, want kind ui without a front-end build", got)
	}

	got.Kind, got.Error = "", ""
	callOK(t, cs, "tango_project_inspect", map[string]any{}, &got)
	if got.Kind != "unknown" || !strings.Contains(got.Error, ConfigFile) {
		t.Errorf("no tan.json: %+v, want kind unknown with an error naming %s", got, ConfigFile)
	}

	if msg := callErr(t, cs, "tango_project_inspect", map[string]any{"dir": "../x"}); !strings.Contains(msg, "outside the MCP root") {
		t.Errorf("inspect ../x: %q, want outside the MCP root", msg)
	}
}

func TestProjectInitNameWithSpaces(t *testing.T) {
	runTestBinaryAsTan(t)
	_, cs := projectServer(t)
	callOK(t, cs, "tango_project_init", map[string]any{"dir": "app", "name": "Hello World"}, nil)
	var got struct {
		Config map[string]any `json:"config"`
	}
	callOK(t, cs, "tango_project_inspect", map[string]any{"dir": "app"}, &got)
	if got.Config["name"] != "Hello World" {
		t.Errorf("config.name = %v, want Hello World", got.Config["name"])
	}
}

// TestToolOutputSchemasAreObjects: TypeScript MCP clients reject a tools/list
// with any output schema that is not an object, which hides every tool.
func TestToolOutputSchemasAreObjects(t *testing.T) {
	_, cs := projectServer(t)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Tools) == 0 {
		t.Fatal("no tools")
	}
	for _, tool := range res.Tools {
		if tool.OutputSchema == nil {
			continue
		}
		b, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatal(err)
		}
		var schema struct {
			Type any `json:"type"`
		}
		if err := json.Unmarshal(b, &schema); err != nil {
			t.Fatal(err)
		}
		if schema.Type != "object" {
			t.Errorf("tool %s: output schema type %v, want object", tool.Name, schema.Type)
		}
	}
}

func TestProjectInitRejectsNonEmpty(t *testing.T) {
	s, cs := projectServer(t)
	if err := os.MkdirAll(filepath.Join(s.root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.root, "app", "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if msg := callErr(t, cs, "tango_project_init", map[string]any{"dir": "app"}); !strings.Contains(msg, "not empty") {
		t.Errorf("init into a non-empty dir: %q, want a not empty error", msg)
	}
}

func TestProjectInitOutsideRoot(t *testing.T) {
	_, cs := projectServer(t)
	if msg := callErr(t, cs, "tango_project_init", map[string]any{"dir": "../x"}); !strings.Contains(msg, "outside the MCP root") {
		t.Errorf("init ../x: %q, want outside the MCP root", msg)
	}
}

func TestProjectInitRejectsBadValues(t *testing.T) {
	_, cs := projectServer(t)
	for _, args := range []map[string]any{
		{"dir": "a", "template": "nope"},
		{"dir": "a", "template": "-local=/x"},
		{"dir": "a", "name": "-yes"},
		{"dir": "a", "name": "Hello\nWorld"},
		{"dir": "a", "module": "x y"},
	} {
		callErr(t, cs, "tango_project_init", args)
	}
}

func TestProjectDoctorShape(t *testing.T) {
	s, cs := projectServer(t)
	// A project at the root whose icon is missing: doctor checks the root,
	// not the server's working directory.
	cfg := `{"name": "Hello", "id": "com.example.hello", "version": "0.1.0", "icon": "missing-icon.png"}`
	if err := os.WriteFile(filepath.Join(s.root, ConfigFile), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	var out struct {
		Checks []struct {
			Name     string `json:"name"`
			Status   string `json:"status"`
			Detail   string `json:"detail"`
			Required bool   `json:"required"`
		} `json:"checks"`
	}
	callOK(t, cs, "tango_project_doctor", nil, &out)
	checks := out.Checks
	if len(checks) == 0 {
		t.Fatal("doctor returned no checks")
	}
	if checks[0].Name != "OS" {
		t.Errorf("first check %q, want OS", checks[0].Name)
	}
	var iconErr bool
	for _, c := range checks {
		if c.Status == "" {
			t.Errorf("check %q has no status", c.Name)
		}
		iconErr = iconErr || (c.Status == "error" && strings.Contains(c.Detail, "missing-icon.png"))
	}
	if !iconErr {
		t.Errorf("no icon error for the project at the root: %+v", checks)
	}
}

func TestProjectBuildRejectsFlagInjection(t *testing.T) {
	_, cs := projectServer(t)
	for _, args := range []map[string]any{
		{"tags": []string{"-ldflags=-X"}},
		{"tags": []string{"a b"}},
		{"platforms": []string{"-o=/tmp"}},
		{"platforms": []string{"darwin/arm64\n-x"}},
	} {
		callErr(t, cs, "tango_project_build", args)
	}
	callErr(t, cs, "tango_project_package", map[string]any{"platforms": []string{"-format=msix"}})
	var jobs []jobInfo
	callOK(t, cs, "tango_jobs", nil, &struct {
		Jobs *[]jobInfo `json:"jobs"`
	}{&jobs})
	if len(jobs) != 0 {
		t.Errorf("jobs after rejected calls: %+v, want none", jobs)
	}
}

// argsScript makes jobs run an sh script that prints its arguments, one
// per line, and $GOFLAGS.
func argsScript(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("runs an sh script as tan")
	}
	path := filepath.Join(t.TempDir(), "tan.sh")
	body := "#!/bin/sh\nfor a in \"$@\"; do echo \"arg:$a\"; done\necho \"GOFLAGS:$GOFLAGS\"\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	old := selfExe
	selfExe = func() (string, error) { return path, nil }
	t.Cleanup(func() { selfExe = old })
}

// waitTool polls tango_job_status until the job ended.
func waitTool(t *testing.T, cs *mcp.ClientSession, id string) jobInfo {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		var info jobInfo
		callOK(t, cs, "tango_job_status", map[string]any{"id": id}, &info)
		if info.State != "running" {
			return info
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s still running: %+v", id, info)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestProjectBuildStartsJob(t *testing.T) {
	argsScript(t)
	s, cs := projectServer(t)
	var info jobInfo
	callOK(t, cs, "tango_project_build", map[string]any{"platforms": []string{"darwin/arm64"}, "tags": []string{"foo"}, "debug": true}, &info)
	if info.Kind != "build" || info.Dir != s.root {
		t.Errorf("job %+v, want a build job in %s", info, s.root)
	}
	done := waitTool(t, cs, info.ID)
	for _, want := range []string{"arg:build\n", "arg:-platform\narg:darwin/arm64\n", "arg:-debug\n", "foo"} {
		if !strings.Contains(done.Output, want) {
			t.Errorf("output %q lacks %q", done.Output, want)
		}
	}
	if !strings.Contains(done.Output, "GOFLAGS:") || !strings.Contains(done.Output, "-tags=") {
		t.Errorf("output %q: tags not passed through GOFLAGS -tags", done.Output)
	}
	var jobs []jobInfo
	callOK(t, cs, "tango_jobs", nil, &struct {
		Jobs *[]jobInfo `json:"jobs"`
	}{&jobs})
	if len(jobs) != 1 || jobs[0].ID != info.ID {
		t.Errorf("tango_jobs = %+v, want the build job", jobs)
	}
	var stopped jobInfo
	callOK(t, cs, "tango_job_stop", map[string]any{"id": info.ID}, &stopped)
	if stopped.State == "running" {
		t.Errorf("after stop: %+v", stopped)
	}
	callErr(t, cs, "tango_job_status", map[string]any{"id": "job-99"})
}

func TestProjectJobArgs(t *testing.T) {
	argsScript(t)
	s, cs := projectServer(t)
	for _, c := range []struct {
		tool string
		args map[string]any
		want []string
	}{
		{"tango_project_package", map[string]any{"platforms": []string{"linux/amd64", "windows/amd64"}}, []string{"package", "-C", s.root, "-platform", "linux/amd64,windows/amd64"}},
		{"tango_project_generate", nil, []string{"generate", "bindings", "-C", s.root}},
		{"tango_project_dev", nil, []string{"dev", "-C", s.root}},
		{"tango_project_build", nil, []string{"build", "-C", s.root}},
	} {
		var info jobInfo
		callOK(t, cs, c.tool, c.args, &info)
		done := waitTool(t, cs, info.ID)
		var got []string
		for _, l := range strings.Split(done.Output, "\n") {
			if a, ok := strings.CutPrefix(l, "arg:"); ok {
				got = append(got, a)
			}
		}
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("%s: tan %q, want %q", c.tool, got, c.want)
		}
	}
}

func TestGoflagsWithTags(t *testing.T) {
	for _, c := range []struct{ goflags, want string }{
		{"", "-tags=foo,bar"},
		{"-mod=mod", "-mod=mod -tags=foo,bar"},
		{"-tags=a,foo -trimpath", "-tags=a,foo -trimpath -tags=a,foo,bar"},
	} {
		if got := goflagsWithTags(c.goflags, []string{"foo", "bar"}); got != c.want {
			t.Errorf("goflagsWithTags(%q) = %q, want %q", c.goflags, got, c.want)
		}
	}
}
