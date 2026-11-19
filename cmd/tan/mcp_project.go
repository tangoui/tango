package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// projectInitTimeout bounds tango_project_init (tan init, go mod tidy included).
const projectInitTimeout = 60 * time.Second

type projectDirInput struct {
	Dir string `json:"dir,omitempty" jsonschema:"project directory, relative to the MCP root (default: the root)"`
}

type projectInspectOutput struct {
	Dir string `json:"dir"`
	// Config is tan.json as tan reads it; absent when it cannot be read.
	Config map[string]any `json:"config,omitempty"`
	Module string         `json:"module,omitempty"`
	// Kind is webview (tan.json has a front end), ui (self-drawn, no front
	// end) or unknown (no readable tan.json; Error says why).
	Kind             string `json:"kind"`
	HasFrontendBuild bool   `json:"hasFrontendBuild"`
	Error            string `json:"error,omitempty"`
}

type doctorCheck struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Required bool   `json:"required"`
}

type projectInitInput struct {
	Dir      string `json:"dir" jsonschema:"directory to create, relative to the MCP root; must not exist or be empty"`
	Template string `json:"template,omitempty" jsonschema:"project template (default vanilla)"`
	Name     string `json:"name,omitempty" jsonschema:"app display name (default: the directory name)"`
	Module   string `json:"module,omitempty" jsonschema:"Go module path (default: derived from the name)"`
}

type projectInitOutput struct {
	Dir    string `json:"dir"`
	Output string `json:"output"`
}

type projectBuildInput struct {
	Dir       string   `json:"dir,omitempty" jsonschema:"project directory, relative to the MCP root (default: the root)"`
	Platforms []string `json:"platforms,omitempty" jsonschema:"targets such as darwin/arm64, windows/amd64, ios, android (default: this machine)"`
	Debug     bool     `json:"debug,omitempty" jsonschema:"debug build: symbols, console, inspector; android: debug APK"`
	Tags      []string `json:"tags,omitempty" jsonschema:"extra Go build tags"`
}

type projectPackageInput struct {
	Dir       string   `json:"dir,omitempty" jsonschema:"project directory, relative to the MCP root (default: the root)"`
	Platforms []string `json:"platforms,omitempty" jsonschema:"targets, as for tango_project_build (default: this machine)"`
}

type jobStatusInput struct {
	ID    string `json:"id" jsonschema:"job id"`
	Lines int    `json:"lines,omitempty" jsonschema:"last output lines to return (default 200; at most 64 KiB)"`
}

type jobIDInput struct {
	ID string `json:"id" jsonschema:"job id"`
}

// addProjectTools registers the tools that inspect, create, build and run
// projects under the root, and those that follow the jobs they start.
func (s *mcpServer) addProjectTools() {
	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_project_inspect",
		Description: "Describe the Tango project in dir: its tan.json, Go module, kind (webview, ui for a self-drawn UI, or unknown without a readable tan.json) and whether the front-end build output exists.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in projectDirInput) (*mcp.CallToolResult, projectInspectOutput, error) {
		dir, err := s.resolve(in.Dir)
		if err != nil {
			return nil, projectInspectOutput{}, err
		}
		return nil, inspectProject(dir), nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_project_doctor",
		Description: "Check the toolchain and platform dependencies (tan doctor): one row per check with its status (ok, missing, warn, or - for optional and absent).",
	}, func(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, []doctorCheck, error) {
		var out []doctorCheck
		for _, c := range doctorChecks() {
			out = append(out, doctorCheck{Name: c.name, Status: string(c.status), Detail: c.detail, Required: c.required})
		}
		return nil, out, nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_project_init",
		Description: "Create a Tango project in dir (tan init) from a template: " + templateNames() + ". Waits for it to finish (at most 60 s) and returns its output.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in projectInitInput) (*mcp.CallToolResult, projectInitOutput, error) {
		out, err := s.initProject(ctx, in)
		return nil, out, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_project_generate",
		Description: "Start tan generate bindings in dir as a background job (TypeScript clients for bind.Register services). Follow it with tango_job_status.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in projectDirInput) (*mcp.CallToolResult, jobInfo, error) {
		info, err := s.startProjectJob("generate", in.Dir, func(dir string) []string {
			return []string{"generate", "bindings", "-C", dir}
		}, nil)
		return nil, info, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_project_build",
		Description: "Start tan build in dir as a background job. Follow it with tango_job_status.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in projectBuildInput) (*mcp.CallToolResult, jobInfo, error) {
		if err := checkArgValues("platforms", in.Platforms); err != nil {
			return nil, jobInfo{}, err
		}
		if err := checkArgValues("tags", in.Tags); err != nil {
			return nil, jobInfo{}, err
		}
		var env []string
		if len(in.Tags) > 0 {
			// tan build has no -tags flag: it takes the tags in GOFLAGS (build_tags.go).
			env = []string{"GOFLAGS=" + goflagsWithTags(os.Getenv("GOFLAGS"), in.Tags)}
		}
		info, err := s.startProjectJob("build", in.Dir, func(dir string) []string {
			args := []string{"build", "-C", dir}
			if len(in.Platforms) > 0 {
				args = append(args, "-platform", strings.Join(in.Platforms, ","))
			}
			if in.Debug {
				args = append(args, "-debug")
			}
			return args
		}, env)
		return nil, info, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_project_package",
		Description: "Start tan package in dir as a background job: a build, then each platform's default installer or archive. Follow it with tango_job_status.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in projectPackageInput) (*mcp.CallToolResult, jobInfo, error) {
		if err := checkArgValues("platforms", in.Platforms); err != nil {
			return nil, jobInfo{}, err
		}
		info, err := s.startProjectJob("package", in.Dir, func(dir string) []string {
			args := []string{"package", "-C", dir}
			if len(in.Platforms) > 0 {
				args = append(args, "-platform", strings.Join(in.Platforms, ","))
			}
			return args
		}, nil)
		return nil, info, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_project_dev",
		Description: "Start tan dev in dir as a background job: the app against the front-end dev server, rebuilt on Go changes. It runs until tango_job_stop.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in projectDirInput) (*mcp.CallToolResult, jobInfo, error) {
		info, err := s.startProjectJob("dev", in.Dir, func(dir string) []string {
			return []string{"dev", "-C", dir}
		}, nil)
		return nil, info, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_jobs",
		Description: "List the background jobs (build, package, dev, generate), oldest first, without output.",
	}, func(context.Context, *mcp.CallToolRequest, any) (*mcp.CallToolResult, []jobInfo, error) {
		return nil, s.jobs.list(), nil
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_job_status",
		Description: "A job's state, exit code and the last lines of its output.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in jobStatusInput) (*mcp.CallToolResult, jobInfo, error) {
		info, err := s.jobs.status(in.ID, in.Lines)
		return nil, info, err
	})

	mcp.AddTool(s.srv, &mcp.Tool{
		Name:        "tango_job_stop",
		Description: "Stop a job and the processes it started (a polite signal, then a kill after 5 s).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, in jobIDInput) (*mcp.CallToolResult, jobInfo, error) {
		info, err := s.jobs.stop(in.ID)
		return nil, info, err
	})
}

// startProjectJob starts "tan args(dir)..." as a background job of kind in
// the project directory rel, which must be a directory under the root.
func (s *mcpServer) startProjectJob(kind, rel string, args func(dir string) []string, env []string) (jobInfo, error) {
	dir, err := s.resolve(rel)
	if err != nil {
		return jobInfo{}, err
	}
	if fi, err := os.Stat(dir); err != nil {
		return jobInfo{}, err
	} else if !fi.IsDir() {
		return jobInfo{}, fmt.Errorf("%s is not a directory", dir)
	}
	return s.jobs.start(kind, dir, args(dir), env)
}

// checkArgValues rejects values that a command line could take for a flag
// (a leading "-") or split (white space): tool inputs never add arguments.
func checkArgValues(field string, values []string) error {
	for _, v := range values {
		if err := checkArgValue(field, v); err != nil {
			return err
		}
	}
	return nil
}

func checkArgValue(field, v string) error {
	if strings.HasPrefix(v, "-") || strings.IndexFunc(v, unicode.IsSpace) >= 0 {
		return fmt.Errorf("%s: %q is not allowed (no leading - or white space)", field, v)
	}
	if v == "" {
		return fmt.Errorf("%s: empty value", field)
	}
	return nil
}

// goflagsWithTags returns goflags with tags added to the build tags it
// sets: the last -tags in GOFLAGS wins, so the old ones are repeated in it.
func goflagsWithTags(goflags string, tags []string) string {
	all := goflagsTags(goflags)
	for _, t := range tags {
		if !slices.Contains(all, t) {
			all = append(all, t)
		}
	}
	return strings.TrimSpace(goflags + " -tags=" + strings.Join(all, ","))
}

// inspectProject describes the project in the absolute directory dir.
func inspectProject(dir string) projectInspectOutput {
	out := projectInspectOutput{Dir: dir, Kind: "unknown", Module: goModulePath(dir)}
	cfg, err := LoadConfig(dir)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	b, err := json.Marshal(cfg)
	if err == nil {
		err = json.Unmarshal(b, &out.Config)
	}
	if err != nil {
		out.Error = err.Error()
	}
	if !cfg.HasFrontend() {
		out.Kind = "ui"
		return out
	}
	out.Kind = "webview"
	if fi, err := os.Stat(cfg.DistDir()); err == nil && fi.IsDir() {
		out.HasFrontendBuild = true
	}
	return out
}

// goModulePath returns the module path in dir/go.mod, or "".
func goModulePath(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "//")
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "module")
		if !ok || rest == "" || !unicode.IsSpace(rune(rest[0])) {
			continue
		}
		p := strings.TrimSpace(rest)
		if u, err := strconv.Unquote(p); err == nil {
			p = u
		}
		return p
	}
	return ""
}

// initProject runs tan init for the tool, without questions, and waits.
func (s *mcpServer) initProject(ctx context.Context, in projectInitInput) (projectInitOutput, error) {
	if in.Dir == "" {
		return projectInitOutput{}, errors.New("dir: required")
	}
	if in.Template == "" {
		in.Template = defaultTemplate
	}
	for _, f := range []struct{ name, v string }{{"template", in.Template}, {"name", in.Name}, {"module", in.Module}} {
		if f.v == "" {
			continue
		}
		if err := checkArgValue(f.name, f.v); err != nil {
			return projectInitOutput{}, err
		}
	}
	if _, ok := findTemplate(in.Template); !ok {
		return projectInitOutput{}, fmt.Errorf("template: unknown template %q (have %s)", in.Template, templateNames())
	}
	dir, err := s.resolve(in.Dir)
	if err != nil {
		return projectInitOutput{}, err
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return projectInitOutput{}, fmt.Errorf("%s exists and is not empty", dir)
	} else if fi, serr := os.Stat(dir); serr == nil && !fi.IsDir() {
		return projectInitOutput{}, fmt.Errorf("%s exists and is not a directory", dir)
	}

	// A directory argument keeps tan init from asking questions; -yes says
	// so again, and stdin is the null device.
	args := []string{"init", "-yes", "-template", in.Template}
	if in.Name != "" {
		args = append(args, "-name", in.Name)
	}
	if in.Module != "" {
		args = append(args, "-module", in.Module)
	}
	args = append(args, dir)
	out, err := runTan(ctx, s.root, args, projectInitTimeout)
	if err != nil {
		return projectInitOutput{}, fmt.Errorf("tan init: %w\n%s", err, out)
	}
	return projectInitOutput{Dir: dir, Output: out}, nil
}

// runTan runs "tan args..." in dir and waits at most timeout for it,
// returning the end of its output. On timeout or cancellation the process
// and its children are killed.
func runTan(ctx context.Context, dir string, args []string, timeout time.Duration) (string, error) {
	exe, err := selfExe()
	if err != nil {
		return "", fmt.Errorf("finding the tan executable: %w", err)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), selfEnv...)
	out := newTailBuffer(jobStatusMaxBytes)
	cmd.Stdout, cmd.Stderr = out, out
	cmd.WaitDelay = 5 * time.Second // a child left holding the output
	prepareJobGroup(cmd)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	group := attachJobGroup(cmd)
	defer group.release()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case err = <-done:
	case <-t.C:
		group.kill()
		<-done
		err = fmt.Errorf("timed out after %v", timeout)
	case <-ctx.Done():
		group.kill()
		<-done
		err = ctx.Err()
	}
	s, _ := out.Tail(0)
	return s, err
}
