package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var desktopHelperName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

func validateDesktopHelpers(helpers []DesktopHelper) []error {
	var errs []error
	seen := map[string]bool{}
	for _, helper := range helpers {
		name := strings.ToLower(helper.Name)
		switch {
		case !desktopHelperName.MatchString(helper.Name):
			errs = append(errs, fmt.Errorf(`"desktopHelpers": invalid executable name %q`, helper.Name))
		case seen[name]:
			errs = append(errs, fmt.Errorf(`"desktopHelpers": %q listed twice`, helper.Name))
		case strings.TrimSpace(helper.Build) == "":
			errs = append(errs, fmt.Errorf(`"desktopHelpers": %q needs a build command`, helper.Name))
		}
		seen[name] = true
	}
	return errs
}

func buildDesktopHelpers(ctx context.Context, cfg *Config, a *artifact) error {
	for _, helper := range cfg.DesktopHelpers {
		name := helper.Name
		dir := filepath.Join(a.dir, "cli")
		switch a.target.OS {
		case "windows":
			name += ".exe"
		case "darwin":
			dir = filepath.Join(a.path, "Contents", "Helpers")
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		output := filepath.Join(dir, name)
		env := []string{"TAN_TARGET_OS=" + a.target.OS, "TAN_TARGET_ARCH=" + a.target.Arch, "TAN_HELPER_OUTPUT=" + output}
		if err := runShell(ctx, cfg.dir, env, helper.Build); err != nil {
			return fmt.Errorf("build helper %s: %w", name, err)
		}
		if info, err := os.Lstat(output); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("helper %s did not produce a nonempty regular file at %s", name, output)
		}
		if err := os.Chmod(output, 0o755); err != nil {
			return err
		}
		if a.target.OS == "darwin" {
			if err := adhocSign(ctx, output); err != nil {
				return err
			}
		}
		a.helpers = append(a.helpers, output)
	}
	return nil
}

func copyDesktopHelpers(helpers []string, dir string) error {
	if len(helpers) == 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, source := range helpers {
		if err := copyFile(source, filepath.Join(dir, filepath.Base(source)), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func linuxHelperPath(cfg *Config, name string) string {
	return path.Join("lib", cfg.Exe(), name)
}
