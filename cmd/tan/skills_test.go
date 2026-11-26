package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

const sourceSkill = "---\nname: tango\nmetadata:\n  tango-version: dev\n---\n\nTango skill\n"

func writeSkillFixture(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatal(err)
	}
}

func readSkillFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func skillFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeSkillFixture(t, filepath.Join(dir, "skills/tango/SKILL.md"), sourceSkill)
	writeSkillFixture(t, filepath.Join(dir, "skills/tango/references/a.md"), "reference\n")
	return dir
}

func syncSkillFixture(t *testing.T, module, version, project string) skillChanges {
	t.Helper()
	changes, err := syncSkill(module, version, project)
	if err != nil {
		t.Fatal(err)
	}
	return changes
}

func TestSyncSkillFresh(t *testing.T) {
	project := t.TempDir()
	changes := syncSkillFixture(t, skillFixture(t), "", project)
	if !slices.Equal(changes.Added, []string{"SKILL.md", "references/a.md"}) {
		t.Fatalf("added %v", changes.Added)
	}
	if got := readSkillFixture(t, filepath.Join(project, ".agents/skills/tango/references/a.md")); got != "reference\n" {
		t.Fatalf("reference = %q", got)
	}
}

func TestSyncSkillStampsVersion(t *testing.T) {
	source := skillFixture(t)
	project := t.TempDir()
	syncSkillFixture(t, source, "v0.4.0", project)
	installed := filepath.Join(project, ".agents/skills/tango/SKILL.md")
	if got := readSkillFixture(t, installed); !strings.Contains(got, "  tango-version: v0.4.0\n") {
		t.Fatalf("published skill version missing in %q", got)
	}
	changes := syncSkillFixture(t, source, "", project)
	if got := readSkillFixture(t, installed); !strings.Contains(got, "  tango-version: local\n") {
		t.Fatalf("local skill version missing in %q", got)
	}
	if !slices.Equal(changes.Modified, []string{"SKILL.md"}) {
		t.Fatalf("modified %v", changes.Modified)
	}
	if got := readSkillFixture(t, filepath.Join(source, "skills/tango/SKILL.md")); got != sourceSkill {
		t.Fatalf("source changed to %q", got)
	}
}

func TestSyncSkillRestoresAndPrunes(t *testing.T) {
	source, project := skillFixture(t), t.TempDir()
	syncSkillFixture(t, source, "", project)
	dir := filepath.Join(project, ".agents/skills/tango")
	ref, extra := filepath.Join(dir, "references/a.md"), filepath.Join(dir, "extra.md")
	writeSkillFixture(t, ref, "changed")
	writeSkillFixture(t, extra, "extra")
	changes := syncSkillFixture(t, source, "", project)
	if !slices.Equal(changes.Modified, []string{"references/a.md"}) || !slices.Equal(changes.Removed, []string{"extra.md"}) {
		t.Fatalf("changes = %+v", changes)
	}
	if got := readSkillFixture(t, ref); got != "reference\n" {
		t.Fatalf("restored reference = %q", got)
	}
	if _, err := os.Stat(extra); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checking removed file: %v", err)
	}
}

func TestSyncSkillKeepsSiblings(t *testing.T) {
	project := t.TempDir()
	other := filepath.Join(project, ".agents/skills/mine/SKILL.md")
	writeSkillFixture(t, other, "my skill")
	syncSkillFixture(t, skillFixture(t), "", project)
	if got := readSkillFixture(t, other); got != "my skill" {
		t.Fatalf("sibling skill = %q", got)
	}
}

func TestSyncSkillIdempotent(t *testing.T) {
	source, project := skillFixture(t), t.TempDir()
	syncSkillFixture(t, source, "", project)
	path := filepath.Join(project, ".agents/skills/tango/SKILL.md")
	past := time.Unix(1600000000, 0)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if changes := syncSkillFixture(t, source, "", project); !changes.Empty() {
		t.Fatalf("changes = %+v", changes)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.ModTime().Equal(past) {
		t.Fatalf("mtime changed to %v", info.ModTime())
	}
}

func TestSyncSkillMissingSource(t *testing.T) {
	project := t.TempDir()
	_, err := syncSkill(t.TempDir(), "", project)
	if err == nil || !strings.Contains(err.Error(), "upgrade github.com/tangoui/tango") {
		t.Fatalf("missing source error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checking project directory: %v", err)
	}
}

func TestSyncSkillRequiresVersionLine(t *testing.T) {
	source := skillFixture(t)
	writeSkillFixture(t, filepath.Join(source, "skills/tango/SKILL.md"), "---\nname: tango\n---\n  tango-version: dev\n")
	if _, err := syncSkill(source, "", t.TempDir()); err == nil {
		t.Fatal("accepted the version line outside frontmatter")
	}
}

func TestSyncSkillStaleStage(t *testing.T) {
	source, project := skillFixture(t), t.TempDir()
	writeSkillFixture(t, filepath.Join(project, ".agents/skills/.tango-update/next/SKILL.md"), "x\n")
	_, err := syncSkill(source, "", project)
	if err == nil || !strings.Contains(err.Error(), "remove "+filepath.Join(project, ".agents", "skills", ".tango-update")) {
		t.Fatalf("stale stage error = %v", err)
	}

	// Interrupted between the renames: the skill is only in previous.
	writeSkillFixture(t, filepath.Join(project, ".agents/skills/.tango-update/previous/SKILL.md"), "old\n")
	_, err = syncSkill(source, "", project)
	if err == nil || !strings.Contains(err.Error(), "move it back to "+filepath.Join(project, ".agents", "skills", "tango")) {
		t.Fatalf("interrupted update error = %v", err)
	}
}

func TestLocateTangoUsesLocalReplacement(t *testing.T) {
	source, project := skillFixture(t), t.TempDir()
	writeSkillFixture(t, filepath.Join(source, "go.mod"), "module github.com/tangoui/tango\n\ngo 1.27\n")
	writeSkillFixture(t, filepath.Join(project, "go.mod"), "module example.com/app\n\ngo 1.27\n\nrequire github.com/tangoui/tango v0.3.0\n\nreplace github.com/tangoui/tango => "+filepath.ToSlash(source)+"\n")
	t.Setenv("GOWORK", "off")
	dir, version, err := locateTango(context.Background(), project)
	if err != nil || dir != source || version != "" {
		t.Fatalf("locateTango = %q, %q, %v", dir, version, err)
	}
}

func TestLocateTangoInVendoredProject(t *testing.T) {
	source, project := skillFixture(t), t.TempDir()
	writeSkillFixture(t, filepath.Join(source, "go.mod"), "module github.com/tangoui/tango\n\ngo 1.27\n")
	writeSkillFixture(t, filepath.Join(project, "go.mod"), "module example.com/app\n\ngo 1.27\n\nrequire github.com/tangoui/tango v0.3.0\n\nreplace github.com/tangoui/tango => "+filepath.ToSlash(source)+"\n")
	writeSkillFixture(t, filepath.Join(project, "vendor/modules.txt"), "# github.com/tangoui/tango v0.3.0 => "+filepath.ToSlash(source)+"\n## explicit; go 1.27\n# github.com/tangoui/tango => "+filepath.ToSlash(source)+"\n")
	t.Setenv("GOWORK", "off")
	dir, _, err := locateTango(context.Background(), project)
	if err != nil || dir != source {
		t.Fatalf("locateTango = %q, %v; want %q", dir, err, source)
	}
}

func skillProjectFixture(t *testing.T) string {
	t.Helper()
	module, project := skillFixture(t), t.TempDir()
	writeSkillFixture(t, filepath.Join(module, "go.mod"), "module github.com/tangoui/tango\n\ngo 1.27\n")
	writeSkillFixture(t, filepath.Join(project, "go.mod"), "module example.com/app\n\ngo 1.27\n\nrequire github.com/tangoui/tango v0.3.0\n\nreplace github.com/tangoui/tango => "+strconv.Quote(filepath.ToSlash(module))+"\n")
	t.Setenv("GOWORK", "off")
	return project
}

func TestRunSkillsUpdate(t *testing.T) {
	project := skillProjectFixture(t)
	update := func() string {
		return captureStd(t, func() {
			if err := runSkills([]string{"update", "-C", project}); err != nil {
				t.Error(err)
			}
		})
	}
	if out := update(); !strings.Contains(out, "tango skill local -> .agents/skills/tango") ||
		!strings.Contains(out, "added 2, modified 0, removed 0") {
		t.Fatalf("install output = %q", out)
	}
	ref := filepath.Join(project, ".agents/skills/tango/references/a.md")
	writeSkillFixture(t, ref, "changed")
	if out := update(); !strings.Contains(out, "added 0, modified 1, removed 0") ||
		!strings.Contains(out, "modified references/a.md") {
		t.Fatalf("update output = %q", out)
	}
	if got := readSkillFixture(t, ref); got != "reference\n" {
		t.Fatalf("restored reference = %q", got)
	}
}

func TestRunSkillsUpdateUpToDate(t *testing.T) {
	project := skillProjectFixture(t)
	for i := 0; i < 2; i++ {
		out := captureStd(t, func() {
			if err := runSkills([]string{"update", "-C", project}); err != nil {
				t.Error(err)
			}
		})
		if i == 1 && !strings.Contains(out, "up to date") {
			t.Fatalf("repeat output = %q", out)
		}
	}
}

func TestRunSkillsUpdateNotATangoProject(t *testing.T) {
	t.Setenv("GOWORK", "off")
	for _, tt := range []struct {
		name, goMod, want string
	}{
		{"missing module file", "", "go.mod"},
		{"missing Tango dependency", "module example.com/app\n\ngo 1.27\n", "github.com/tangoui/tango"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			project := t.TempDir()
			if tt.goMod != "" {
				writeSkillFixture(t, filepath.Join(project, "go.mod"), tt.goMod)
			}
			if err := runSkills([]string{"update", "-C", project}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("update error = %v, want %q", err, tt.want)
			}
			if _, err := os.Stat(filepath.Join(project, ".agents")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unexpected .agents: %v", err)
			}
		})
	}
}

func TestRunSkillsUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}} {
		out := captureStd(t, func() {
			if err := runSkills(args); err == nil {
				t.Error("expected usage error")
			}
		})
		if !strings.Contains(out, "Usage: tan skills update") {
			t.Fatalf("usage output = %q", out)
		}
	}
}
