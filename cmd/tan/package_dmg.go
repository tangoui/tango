package main

// The macOS disk image: the app, a link to /Applications, a laid-out
// Finder window (.DS_Store from dsstore.go), an optional background in
// the hidden .background folder, and the app's icon as the volume icon.
//
// hdiutil makes a read-write image from a staging folder; tan mounts it
// without showing it in Finder (-nobrowse), writes .DS_Store (whose
// background alias needs the files' catalog ids on the mounted volume),
// sets the volume's custom-icon flag, detaches, and converts the image to
// a read-only LZMA-compressed one (ULMO).

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// dmgLayout is DMGLayout with the defaults filled in.
type dmgLayout struct {
	title                string
	width, height        int
	iconSize             int
	appX, appY           int
	applicationsX, applY int
	background           string // project file, or ""
}

func dmgLayoutFor(cfg *Config) dmgLayout {
	d := cfg.Installer.MacOS.DMG
	l := dmgLayout{
		title: cmp.Or(d.Title, cfg.Name), width: 540, height: 380, iconSize: cmp.Or(d.IconSize, 128),
		appX: 140, appY: 190, applicationsX: 400, applY: 190,
	}
	if len(d.WindowSize) == 2 {
		l.width, l.height = d.WindowSize[0], d.WindowSize[1]
	}
	if len(d.AppPosition) == 2 {
		l.appX, l.appY = d.AppPosition[0], d.AppPosition[1]
	}
	if len(d.ApplicationsPosition) == 2 {
		l.applicationsX, l.applY = d.ApplicationsPosition[0], d.ApplicationsPosition[1]
	}
	if d.Background != "" {
		l.background = cfg.Path(d.Background)
	}
	return l
}

// dmgWindowOrigin is where the window opens, in screen points from the
// bottom left (Finder keeps it on screen).
const dmgWindowX, dmgWindowY = 200, 120

// dmgStoreRecords are the .DS_Store records: the window (bwsp), the icon
// view (icvp) with the background alias when there is one, and the icon
// positions. Hidden files, should the user show them, sit off to the
// right.
func dmgStoreRecords(l dmgLayout, appName string, bgAlias []byte) ([]dsRecord, error) {
	bwsp, err := encodeBplist(map[string]any{
		"ContainerShowSidebar":  false,
		"PreviewPaneVisibility": false,
		"ShowPathbar":           false,
		"ShowSidebar":           false,
		"ShowStatusBar":         false,
		"ShowTabView":           false,
		"ShowToolbar":           false,
		"SidebarWidth":          0,
		// The content size: Finder adds the title bar.
		"WindowBounds": fmt.Sprintf("{{%d, %d}, {%d, %d}}", dmgWindowX, dmgWindowY, l.width, l.height),
	})
	if err != nil {
		return nil, err
	}
	view := map[string]any{
		"viewOptionsVersion":   1,
		"arrangeBy":            "none",
		"iconSize":             float64(l.iconSize),
		"textSize":             float64(13),
		"gridSpacing":          float64(100),
		"gridOffsetX":          float64(0),
		"gridOffsetY":          float64(0),
		"labelOnBottom":        true,
		"showIconPreview":      true,
		"showItemInfo":         false,
		"backgroundType":       0,
		"backgroundColorRed":   float64(1),
		"backgroundColorGreen": float64(1),
		"backgroundColorBlue":  float64(1),
	}
	if bgAlias != nil {
		view["backgroundType"] = 2 // picture
		view["backgroundImageAlias"] = bgAlias
	}
	icvp, err := encodeBplist(view)
	if err != nil {
		return nil, err
	}
	off := l.width + 200
	return []dsRecord{
		dsBlob(".", "bwsp", bwsp),
		dsBlob(".", "icvp", icvp),
		dsType(".", "vstl", "icnv"),
		dsLong(".", "vSrn", 1),
		dsIloc(appName, l.appX, l.appY),
		dsIloc("Applications", l.applicationsX, l.applY),
		dsIloc(".background", off, 50),
		dsIloc(".VolumeIcon.icns", off, 150),
		dsIloc(".fseventsd", off, 250),
		dsIloc(".DS_Store", off, 350),
	}, nil
}

// packageDarwin makes <base>.dmg. hdiutil exists only on macOS.
func packageDarwin(ctx context.Context, cfg *Config, a artifact) ([]string, error) {
	if runtime.GOOS != "darwin" {
		warnf("Disk images can only be made on macOS; skipping %s", a.target)
		return nil, nil
	}
	l := dmgLayoutFor(cfg)
	work, err := os.MkdirTemp("", "tango-dmg-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	stage := filepath.Join(work, "stage")
	appName := filepath.Base(a.path)
	// ditto keeps bundle metadata and signatures intact.
	if err := runIn(ctx, "", nil, "ditto", a.path, filepath.Join(stage, appName)); err != nil {
		return nil, err
	}
	if err := os.Symlink("/Applications", filepath.Join(stage, "Applications")); err != nil {
		return nil, err
	}
	bgName, err := stageDMGBackground(ctx, l.background, filepath.Join(stage, ".background"))
	if err != nil {
		return nil, err
	}
	icns := filepath.Join(a.path, "Contents", "Resources", "icon.icns")
	hasIcon := false
	if _, err := os.Stat(icns); err == nil {
		if err := copyFile(icns, filepath.Join(stage, ".VolumeIcon.icns"), 0o644); err != nil {
			return nil, err
		}
		hasIcon = true
	}

	rw := filepath.Join(work, "rw.dmg")
	size, err := dirSize(stage)
	if err != nil {
		return nil, err
	}
	// Room for .DS_Store and the file system's own structures.
	mb := size/(1<<20)*11/10 + 16
	if err := runIn(ctx, "", nil, "hdiutil", "create", "-quiet", "-volname", l.title, "-srcfolder", stage,
		"-fs", "HFS+", "-format", "UDRW", "-size", fmt.Sprintf("%dm", mb), "-ov", rw); err != nil {
		return nil, err
	}
	mnt := filepath.Join(work, "mnt")
	if err := os.Mkdir(mnt, 0o755); err != nil {
		return nil, err
	}
	if err := runIn(ctx, "", nil, "hdiutil", "attach", "-quiet", "-nobrowse", "-noautoopen", "-noverify", "-readwrite", "-mountpoint", mnt, rw); err != nil {
		return nil, err
	}
	err = finishDMGVolume(mnt, l, appName, bgName, hasIcon)
	if derr := detachDMG(ctx, mnt); err == nil {
		err = derr
	}
	if err != nil {
		return nil, err
	}

	dmg := filepath.Join(filepath.Dir(a.dir), artifactBase(cfg, a.target)+".dmg")
	os.Remove(dmg)
	// ULMO (LZMA) is the smallest read-only format hdiutil offers, about a
	// quarter smaller than UDZO for a Go app; it mounts on macOS 10.15 and
	// later, below the LSMinimumSystemVersion tan writes (11.0).
	if err := runIn(ctx, "", nil, "hdiutil", "convert", "-quiet", rw, "-format", "ULMO", "-ov", "-o", dmg); err != nil {
		return nil, err
	}
	return []string{dmg}, nil
}

// stageDMGBackground copies the background into dir as background.<ext>,
// or, when x@2x.<ext> sits beside x.<ext>, joins both into a
// multi-resolution background.tiff with tiffutil. It returns the file
// name, or "" without a background.
func stageDMGBackground(ctx context.Context, src, dir string) (string, error) {
	if src == "" {
		return "", nil
	}
	if _, err := os.Stat(src); err != nil {
		return "", fmt.Errorf(`"installer.macos.dmg.background": %w`, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ext := filepath.Ext(src)
	hidpi := strings.TrimSuffix(src, ext) + "@2x" + ext
	if _, err := os.Stat(hidpi); err == nil && !strings.EqualFold(ext, ".tiff") && !strings.EqualFold(ext, ".tif") {
		if _, err := exec.LookPath("tiffutil"); err == nil {
			name := "background.tiff"
			if err := runIn(ctx, "", nil, "tiffutil", "-cathidpicheck", src, hidpi, "-out", filepath.Join(dir, name)); err != nil {
				return "", err
			}
			return name, nil
		}
		warnf("tiffutil is missing: the disk image uses %s without its @2x version", filepath.Base(src))
	}
	name := "background" + strings.ToLower(ext)
	return name, copyFile(src, filepath.Join(dir, name), 0o644)
}

// finishDMGVolume writes .DS_Store on the mounted volume, marks it as
// having a custom icon, hides the support files, and removes the
// file-system event log the mount created.
func finishDMGVolume(mnt string, l dmgLayout, appName, bgName string, hasIcon bool) error {
	var alias []byte
	if bgName != "" {
		bg := filepath.Join(mnt, ".background", bgName)
		fileID, fileDate, err := catalogInfo(bg)
		if err != nil {
			return err
		}
		parentID, _, err := catalogInfo(filepath.Dir(bg))
		if err != nil {
			return err
		}
		_, volDate, err := catalogInfo(mnt)
		if err != nil {
			return err
		}
		alias = encodeMacAlias(macAliasTarget{
			volumeName: l.title, volumeDate: volDate, mountPoint: "/Volumes/" + l.title,
			path: "/.background/" + bgName, parentID: parentID, fileID: fileID, fileDate: fileDate,
			ejectable: true,
		})
	}
	records, err := dmgStoreRecords(l, appName, alias)
	if err != nil {
		return err
	}
	store, err := encodeDSStore(records)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(mnt, ".DS_Store"), store, 0o644); err != nil {
		return err
	}
	if hasIcon {
		if err := setCustomIconFlag(mnt); err != nil {
			return fmt.Errorf("volume icon: %w", err)
		}
	}
	for _, f := range []string{".background", ".VolumeIcon.icns", ".DS_Store"} {
		setHidden(filepath.Join(mnt, f)) // best effort: dot files are hidden anyway
	}
	os.RemoveAll(filepath.Join(mnt, ".fseventsd"))
	return nil
}

// detachDMG unmounts the image, forcing it after a busy first try.
func detachDMG(ctx context.Context, mnt string) error {
	var err error
	for i := range 3 {
		args := []string{"detach", "-quiet", mnt}
		if i == 2 {
			args = append(args, "-force")
		}
		if err = runIn(ctx, "", nil, "hdiutil", args...); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}

// dirSize is the total size of the regular files under dir.
func dirSize(dir string) (int64, error) {
	var n int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			n += info.Size()
		}
		return nil
	})
	return n, err
}
