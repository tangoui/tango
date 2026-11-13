package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Debian binary packages, written without dpkg-deb. The format is deb(5):
// an ar(5) archive holding, in this order, debian-binary ("2.0\n"),
// control.tar.gz and data.tar.gz. The control archive has the control file
// (deb-control(5), fields per Debian Policy chapter 5), md5sums and the
// maintainer scripts; the data archive the installed tree, paths relative
// to / as "./usr/...".

// debDefaultDepends are the C library the program is linked against and
// the libraries the Tango runtime loads with dlopen
// (internal/platform/linux): GTK 3 and WebKitGTK 4.1.
var debDefaultDepends = []string{"libc6", "libgtk-3-0", "libwebkit2gtk-4.1-0"}

// debCopyright is /usr/share/doc/<package>/copyright (Policy 12.5), in
// plain form: the name, homepage, copyright and license from tan.json.
func debCopyright(cfg *Config) []byte {
	var b strings.Builder
	b.WriteString(cfg.Name + "\n")
	if cfg.Linux.Homepage != "" {
		b.WriteString("Homepage: " + cfg.Linux.Homepage + "\n")
	}
	if cfg.Copyright != "" {
		b.WriteString("\n" + cfg.Copyright + "\n")
	}
	if cfg.Linux.License != "" {
		b.WriteString("\nLicense: " + cfg.Linux.License + "\n")
	}
	return []byte(b.String())
}

// debArch maps GOARCH to the Debian architecture name.
func debArch(goarch string) string {
	switch goarch {
	case "386":
		return "i386"
	case "arm":
		return "armhf"
	}
	return goarch // amd64, arm64, riscv64, ppc64le, s390x are the same
}

// debControl renders the control file. installedKB is Installed-Size.
func debControl(cfg *Config, arch string, installedKB int64) string {
	var b strings.Builder
	field := func(name, value string) {
		if value = oneLine(value); value != "" {
			fmt.Fprintf(&b, "%s: %s\n", name, value)
		}
	}
	field("Package", linuxPackageName(cfg))
	field("Version", linuxPackageVersion(cfg.Version))
	field("Architecture", arch)
	field("Maintainer", cfg.Linux.Maintainer)
	field("Installed-Size", fmt.Sprint(installedKB))
	field("Depends", strings.Join(append(debDefaultDepends[:len(debDefaultDepends):len(debDefaultDepends)], cfg.Linux.Depends...), ", "))
	field("Section", "misc")
	field("Priority", "optional")
	field("Homepage", cfg.Linux.Homepage)
	// Policy 5.6.13: the synopsis on the field line, then the extended
	// description, each line indented by a space, an empty line as " .".
	b.WriteString("Description: " + oneLine(linuxSummary(cfg)) + "\n")
	if long := linuxDescription(cfg); long != "" {
		for line := range strings.SplitSeq(long, "\n") {
			if line = strings.TrimRight(oneLine(line), " "); line == "" {
				b.WriteString(" .\n")
			} else {
				b.WriteString(" " + line + "\n")
			}
		}
	}
	return b.String()
}

// packageDeb writes <base>.deb next to the build directory.
func packageDeb(cfg *Config, a artifact, icons map[string][]byte) (string, error) {
	files, err := linuxSystemFiles(cfg, a, icons)
	if err != nil {
		return "", err
	}
	files = append(files, sysFile{path: "/usr/share/doc/" + linuxPackageName(cfg) + "/copyright", data: debCopyright(cfg), mode: 0o644})
	slices.SortFunc(files, func(a, b sysFile) int { return strings.Compare(a.path, b.path) })
	if cfg.Linux.Maintainer == "" {
		warnf(`deb: Debian Policy requires a Maintainer; set "linux.maintainer" in %s, like "Jane Doe <jane@example.com>"`, ConfigFile)
	}
	mtime := packageTime()
	var size int64
	var sums strings.Builder
	for _, f := range files {
		size += int64(len(f.data))
		fmt.Fprintf(&sums, "%x  %s\n", md5.Sum(f.data), strings.TrimPrefix(f.path, "/"))
	}
	// Installed-Size: KiB, rounded up (Policy 5.6.20).
	installedKB := (size + 1023) / 1024
	script := []byte(linuxCacheScript(files))

	control, err := debTarGz(mtime, nil, []debTarFile{
		{name: "control", data: []byte(debControl(cfg, debArch(a.target.Arch), installedKB)), mode: 0o644},
		{name: "md5sums", data: []byte(sums.String()), mode: 0o644},
		{name: "postinst", data: script, mode: 0o755},
		{name: "postrm", data: script, mode: 0o755},
	})
	if err != nil {
		return "", err
	}
	var dataFiles []debTarFile
	for _, f := range files {
		dataFiles = append(dataFiles, debTarFile{name: strings.TrimPrefix(f.path, "/"), data: f.data, mode: f.mode})
	}
	var dirs []string
	for _, d := range linuxParentDirs(files) {
		dirs = append(dirs, strings.TrimPrefix(d, "/"))
	}
	data, err := debTarGz(mtime, dirs, dataFiles)
	if err != nil {
		return "", err
	}

	out := filepath.Join(filepath.Dir(a.dir), artifactBase(cfg, a.target)+".deb")
	f, err := os.Create(out)
	if err != nil {
		return "", err
	}
	defer f.Close()
	err = writeAr(f, mtime, []arMember{
		{name: "debian-binary", data: []byte("2.0\n")},
		{name: "control.tar.gz", data: control},
		{name: "data.tar.gz", data: data},
	})
	if err != nil {
		return "", err
	}
	return out, f.Close()
}

type debTarFile struct {
	name string // relative, without "./"
	data []byte
	mode int64
}

// debTarGz writes a gzip-compressed tar of "./", the directories dirs
// (parents first) and files, all owned by root.
func debTarGz(mtime time.Time, dirs []string, files []debTarFile) ([]byte, error) {
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(gz)
	hdr := func(name string, typ byte, mode, size int64) *tar.Header {
		return &tar.Header{Name: name, Typeflag: typ, Mode: mode, Size: size, ModTime: mtime,
			Uname: "root", Gname: "root", Format: tar.FormatGNU}
	}
	if err := tw.WriteHeader(hdr("./", tar.TypeDir, 0o755, 0)); err != nil {
		return nil, err
	}
	for _, d := range dirs {
		if err := tw.WriteHeader(hdr("./"+d+"/", tar.TypeDir, 0o755, 0)); err != nil {
			return nil, err
		}
	}
	for _, f := range files {
		if err := tw.WriteHeader(hdr("./"+f.name, tar.TypeReg, f.mode, int64(len(f.data)))); err != nil {
			return nil, err
		}
		if _, err := tw.Write(f.data); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type arMember struct {
	name string // at most 16 bytes
	data []byte
}

// writeAr writes a common ar(5) archive: the global header "!<arch>\n",
// then per member a 60-byte header (name, mtime, uid, gid, octal mode,
// size, "`\n"; fields space-padded) and the data, padded to an even
// length with "\n". deb(5) uses member names without a trailing '/'.
func writeAr(w io.Writer, mtime time.Time, members []arMember) error {
	if _, err := io.WriteString(w, "!<arch>\n"); err != nil {
		return err
	}
	for _, m := range members {
		if len(m.name) > 16 {
			return fmt.Errorf("ar member name %q is longer than 16 bytes", m.name)
		}
		h := fmt.Sprintf("%-16s%-12d%-6d%-6d%-8o%-10d`\n", m.name, mtime.Unix(), 0, 0, 0o100644, len(m.data))
		if _, err := io.WriteString(w, h); err != nil {
			return err
		}
		if _, err := w.Write(m.data); err != nil {
			return err
		}
		if len(m.data)%2 == 1 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}
