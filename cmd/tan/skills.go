package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const skillDir = ".agents/skills/tango"

func locateTango(ctx context.Context, projectDir string) (dir, version string, err error) {
	// A replacement has its own version. A local replacement has none,
	// even when the original module has a version in the require directive.
	const format = `{{.Dir}}{{"\t"}}{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}`
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", format, tangoModule)
	cmd.Dir = projectDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("locating %s from %s: %w: %s", tangoModule, projectDir, err, strings.TrimSpace(stderr.String()))
	}
	dir, version, _ = strings.Cut(strings.TrimRight(string(out), "\r\n"), "\t")
	if dir == "" {
		return "", "", fmt.Errorf("locating %s from %s: module not downloaded (run go mod download)", tangoModule, projectDir)
	}
	return dir, version, nil
}

func skillVersion(version string) string {
	if version == "" {
		return "local"
	}
	return version
}

type skillChanges struct {
	Added, Modified, Removed []string
}

func (c skillChanges) Empty() bool {
	return len(c.Added)+len(c.Modified)+len(c.Removed) == 0
}

func syncSkill(moduleDir, version, projectDir string) (skillChanges, error) {
	sourceDir := filepath.Join(moduleDir, "skills", "tango")
	source := os.DirFS(sourceDir)
	if _, err := fs.Stat(source, "SKILL.md"); errors.Is(err, fs.ErrNotExist) {
		return skillChanges{}, fmt.Errorf("%s has no skills/tango/SKILL.md; upgrade %s", moduleDir, tangoModule)
	} else if err != nil {
		return skillChanges{}, err
	}
	files, err := readSkillFiles(source, true)
	if err != nil {
		return skillChanges{}, err
	}
	files["SKILL.md"], err = stampSkillVersion(files["SKILL.md"], skillVersion(version))
	if err != nil {
		return skillChanges{}, err
	}

	project, err := os.OpenRoot(projectDir)
	if err != nil {
		return skillChanges{}, err
	}
	defer project.Close()
	parent := path.Dir(skillDir)
	for _, dir := range []string{path.Dir(parent), parent} {
		if err := project.Mkdir(dir, 0755); err != nil && !errors.Is(err, fs.ErrExist) {
			return skillChanges{}, err
		}
		info, err := project.Lstat(dir)
		if err != nil {
			return skillChanges{}, err
		}
		if !info.IsDir() {
			return skillChanges{}, fmt.Errorf("%s must be a directory", dir)
		}
	}

	// The staging directory also serializes concurrent updates. Keep the
	// previous installation there until the replacement is in place.
	stage := path.Join(parent, ".tango-update")
	if err := project.Mkdir(stage, 0700); err != nil {
		return skillChanges{}, fmt.Errorf("preparing Tango skill update: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = project.RemoveAll(stage)
		}
	}()

	var installed map[string][]byte
	exists := false
	if info, err := project.Lstat(skillDir); err == nil {
		if !info.IsDir() {
			return skillChanges{}, fmt.Errorf("%s must be a directory", skillDir)
		}
		exists = true
		skill, err := project.OpenRoot(skillDir)
		if err != nil {
			return skillChanges{}, err
		}
		installed, err = readSkillFiles(skill.FS(), false)
		skill.Close()
		if err != nil {
			return skillChanges{}, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return skillChanges{}, err
	}
	changes := compareSkillFiles(files, installed)
	if changes.Empty() {
		return changes, nil
	}

	next, previous := path.Join(stage, "next"), path.Join(stage, "previous")
	for name, contents := range files {
		dest := path.Join(next, name)
		if err := project.MkdirAll(path.Dir(dest), 0755); err != nil {
			return skillChanges{}, err
		}
		if err := project.WriteFile(dest, contents, 0644); err != nil {
			return skillChanges{}, err
		}
	}
	if exists {
		if err := project.Rename(skillDir, previous); err != nil {
			return skillChanges{}, err
		}
	}
	if err := project.Rename(next, skillDir); err != nil {
		if exists {
			if restoreErr := project.Rename(previous, skillDir); restoreErr != nil {
				cleanup = false
				return skillChanges{}, fmt.Errorf("installing skill: %w; restoring previous skill from %s: %w", err, previous, restoreErr)
			}
		}
		return skillChanges{}, err
	}
	return changes, nil
}

func stampSkillVersion(data []byte, version string) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	if strings.TrimSuffix(lines[0], "\r") == "---" {
		for i, line := range lines[1:] {
			if strings.TrimSuffix(line, "\r") == "---" {
				break
			}
			trimmed := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trimmed, "tango-version:") {
				ending := ""
				if strings.HasSuffix(line, "\r") {
					ending = "\r"
				}
				lines[i+1] = line[:len(line)-len(trimmed)] + "tango-version: " + version + ending
				return []byte(strings.Join(lines, "\n")), nil
			}
		}
	}
	return nil, errors.New("SKILL.md frontmatter is missing tango-version")
}

func readSkillFiles(tree fs.FS, source bool) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := fs.WalkDir(tree, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if !source && name != "." {
				entries, err := fs.ReadDir(tree, name)
				if err != nil {
					return err
				}
				if len(entries) == 0 {
					files[name+"/"] = nil
				}
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			if source {
				return fmt.Errorf("skill source %s is not a regular file", name)
			}
			files[name] = nil // A local symlink or special file is replaced, never read.
			return nil
		}
		data, err := fs.ReadFile(tree, name)
		files[name] = data
		return err
	})
	return files, err
}

func compareSkillFiles(wanted, installed map[string][]byte) skillChanges {
	var changes skillChanges
	for name, data := range wanted {
		old, ok := installed[name]
		switch {
		case !ok:
			changes.Added = append(changes.Added, name)
		case old == nil || !bytes.Equal(old, data):
			changes.Modified = append(changes.Modified, name)
		}
	}
	for name := range installed {
		if _, keep := wanted[name]; !keep {
			changes.Removed = append(changes.Removed, name)
		}
	}
	slices.Sort(changes.Added)
	slices.Sort(changes.Modified)
	slices.Sort(changes.Removed)
	return changes
}
