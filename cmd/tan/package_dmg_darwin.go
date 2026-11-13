package main

import (
	"encoding/binary"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// catalogInfo returns a file's catalog node id (its inode number on HFS+)
// and creation date, for an alias.
func catalogInfo(path string) (uint32, time.Time, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0, time.Time{}, &os.PathError{Op: "stat", Path: path, Err: err}
	}
	return uint32(st.Ino), time.Unix(st.Btim.Unix()), nil
}

// finderInfoAttr is the extended attribute holding the 32-byte Finder
// info; its flags are the big-endian uint16 at offset 8, and
// kHasCustomIcon is 0x0400.
const (
	finderInfoAttr = "com.apple.FinderInfo"
	hasCustomIcon  = 0x0400
)

// setCustomIconFlag tells Finder to show dir's .VolumeIcon.icns (for a
// volume root) or its Icon\r file.
func setCustomIconFlag(dir string) error {
	info := make([]byte, 32)
	if n, err := unix.Getxattr(dir, finderInfoAttr, info); err != nil && !errors.Is(err, unix.ENOATTR) {
		return err
	} else if err == nil && n != 32 {
		info = make([]byte, 32)
	}
	flags := binary.BigEndian.Uint16(info[8:])
	binary.BigEndian.PutUint16(info[8:], flags|hasCustomIcon)
	return unix.Setxattr(dir, finderInfoAttr, info, 0)
}

// ufHidden is UF_HIDDEN from <sys/stat.h>.
const ufHidden = 0x8000

// setHidden sets the UF_HIDDEN flag Finder honours.
func setHidden(path string) error {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return err
	}
	return unix.Chflags(path, int(st.Flags)|ufHidden)
}
