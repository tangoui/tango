package main

// makensis for tan package. On Windows hosts without NSIS, tan downloads a
// pinned release of the official NSIS zip from SourceForge, checks its
// SHA-256, and unpacks it under the user cache directory; later runs reuse
// it. macOS and Linux users install NSIS with their package manager.

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The pinned NSIS release. The SHA-256 was computed from the download,
// whose MD5 matches the one SourceForge publishes for the file.
const (
	nsisVersion = "3.13"
	nsisSHA256  = "ba63dffc4410ee89193e1cb5a41989991bd77c61068da17e3156d136b7b0b3d8"
)

// nsisURL and nsisSum are variables so tests can serve their own zip.
var (
	nsisURL = "https://downloads.sourceforge.net/project/nsis/NSIS%203/" + nsisVersion + "/nsis-" + nsisVersion + ".zip"
	nsisSum = nsisSHA256
)

// nsisDownloadTimeout bounds the whole download.
const nsisDownloadTimeout = 5 * time.Minute

// nsisMaxZip caps the download (the zip is about 2.4 MB).
const nsisMaxZip = 64 << 20

// findMakensis returns makensis from PATH or, on Windows, from the cache
// (downloading NSIS into it the first time).
func findMakensis(ctx context.Context) (string, error) {
	if p, err := exec.LookPath("makensis"); err == nil {
		return p, nil
	}
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("makensis is not on PATH (%s)", makensisHint(runtime.GOOS))
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("makensis is not on PATH and there is no cache directory to download NSIS into: %w", err)
	}
	return cachedMakensis(ctx, http.DefaultClient, filepath.Join(cache, "tango", "nsis"))
}

// cachedMakensis returns <dir>/nsis-<version>/makensis.exe, downloading
// and unpacking nsisURL first when it is missing or unfinished.
func cachedMakensis(ctx context.Context, client *http.Client, dir string) (string, error) {
	root := filepath.Join(dir, "nsis-"+nsisVersion)
	exe := filepath.Join(root, "makensis.exe")
	// The marker exists while root is being unpacked. Unpacking in place
	// avoids renaming a directory that a Windows scanner may hold open.
	partial := root + ".partial"
	if exe, ok, err := installedMakensis(exe, partial); ok || err != nil {
		return exe, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Another tan may be filling the same cache: hold the lock while
	// installing, then check again in case it finished while we waited.
	unlock, err := lockDevPublication(ctx, dir)
	if err != nil {
		return "", err
	}
	defer unlock()
	if exe, ok, err := installedMakensis(exe, partial); ok || err != nil {
		return exe, err
	}
	logf("Downloading NSIS %s (makensis is not on PATH)", nsisVersion)
	zipFile, err := downloadVerified(ctx, client, nsisURL, nsisSum, dir)
	if err != nil {
		return "", fmt.Errorf("download NSIS: %w", err)
	}
	defer os.Remove(zipFile)
	if err := os.WriteFile(partial, nil, 0o644); err != nil {
		return "", err
	}
	if err := os.RemoveAll(root); err != nil {
		return "", err
	}
	if err := unzipTo(zipFile, dir); err != nil {
		return "", fmt.Errorf("unpack NSIS: %w", err)
	}
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("unpack NSIS: no nsis-%s/makensis.exe in the zip", nsisVersion)
	}
	if err := os.Remove(partial); err != nil {
		return "", err
	}
	return exe, nil
}

// installedMakensis reports whether exe is a finished install: it exists
// and no partial marker says its directory is still being unpacked.
func installedMakensis(exe, partial string) (string, bool, error) {
	if _, err := os.Stat(partial); err == nil {
		return "", false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	if _, err := os.Stat(exe); err != nil {
		return "", false, nil
	}
	return exe, true, nil
}

// downloadVerified downloads url into a temporary file in dir and returns
// its path when its SHA-256 is sum.
func downloadVerified(ctx context.Context, client *http.Client, url, sum, dir string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, nsisDownloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", url, resp.Status)
	}
	f, err := os.CreateTemp(dir, "download-*.zip")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, nsisMaxZip+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > nsisMaxZip {
		err = fmt.Errorf("%s is larger than %d bytes", url, nsisMaxZip)
	}
	if got := hex.EncodeToString(h.Sum(nil)); err == nil && !strings.EqualFold(got, sum) {
		err = fmt.Errorf("%s: SHA-256 %s, want %s", url, got, sum)
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// unzipTo extracts a zip into dir, refusing entries that leave it.
func unzipTo(zipFile, dir string) error {
	zr, err := zip.OpenReader(zipFile)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("zip entry %q leaves the directory", f.Name)
		}
		dst := filepath.Join(dir, name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
			continue
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("zip entry %q is not a regular file", f.Name)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := extractZipFile(f, dst); err != nil {
			return err
		}
	}
	return nil
}

func extractZipFile(f *zip.File, dst string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil {
		out.Close()
		return errors.Join(err, os.Remove(dst))
	}
	return out.Close()
}
