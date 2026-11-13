package updater

import (
	"bufio"
	"bytes"
	"compress/flate"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"
)

// Binary patches.
//
// Diff implements the approach of Colin Percival's "Naive differences of
// executable code" (2003), the idea behind bsdiff, written for this
// package: a new version of a program is mostly the old one with small,
// regular changes (shifted addresses, renumbered offsets), so the new file
// is described as runs that copy the old file with a bytewise difference
// added — the differences are mostly zeros and compress very well — and
// runs of literal bytes. Matches are found through a suffix array of the
// old file, and each match is grown forwards and backwards with
// "approximate" extension, accepting a run while at least half of its
// bytes equal the old ones.
//
// Patch format (all integers little-endian):
//
//	"TGOPATCH" magic, 8 bytes
//	u64 old size, u64 new size
//	[32]byte SHA-256 of the old file, [32]byte SHA-256 of the new file
//	u64 ×3 compressed lengths of the control, diff and extra streams
//	the three streams, each DEFLATE-compressed
//
// The control stream is a sequence of signed varint triples (add, copy,
// seek): add bytes of old+diff, then copy bytes from the extra stream,
// then move the old position by seek.

const patchMagic = "TGOPATCH"

const patchHeader = 8 + 8 + 8 + 32 + 32 + 3*8

// maxPatchOutput bounds the size a patch may claim to produce.
const maxPatchOutput = 2 << 30

// ErrPatchBase is returned by Patch when the patch was made from a file
// other than old.
var ErrPatchBase = errors.New("updater: patch does not apply to this file")

// Diff returns a patch that turns old into new.
func Diff(old, new []byte) ([]byte, error) {
	var ctrl, dif, extra bytes.Buffer
	sa := suffixArray(old)
	var vb []byte
	emit := func(add, copy, seek int) {
		vb = binary.AppendVarint(vb[:0], int64(add))
		vb = binary.AppendVarint(vb, int64(copy))
		vb = binary.AppendVarint(vb, int64(seek))
		ctrl.Write(vb)
	}

	var scan, length, pos int
	var lastScan, lastPos, lastOffset int
	inOld := func(i int) bool { return i >= 0 && i < len(old) }
	for scan < len(new) {
		oldScore := 0
		scan += length
		// Advance until a match is found that is not merely the
		// continuation of the current alignment, or one that beats it by
		// more than 8 bytes.
		for scsc := scan; scan < len(new); scan++ {
			pos, length = longestMatch(sa, old, new[scan:])
			for ; scsc < scan+length; scsc++ {
				if inOld(scsc+lastOffset) && old[scsc+lastOffset] == new[scsc] {
					oldScore++
				}
			}
			if length == oldScore && length != 0 || length > oldScore+8 {
				break
			}
			if inOld(scan+lastOffset) && old[scan+lastOffset] == new[scan] {
				oldScore--
			}
		}
		if length == oldScore && scan != len(new) {
			continue
		}

		// Grow the previous match forwards from lastScan...
		lenF := 0
		for i, s, best := 0, 0, 0; lastScan+i < scan && lastPos+i < len(old); {
			if old[lastPos+i] == new[lastScan+i] {
				s++
			}
			i++
			if 2*s-i > 2*best-lenF {
				best, lenF = s, i
			}
		}
		// ...and the new one backwards from scan.
		lenB := 0
		if scan < len(new) {
			for i, s, best := 1, 0, 0; scan >= lastScan+i && pos >= i; i++ {
				if old[pos-i] == new[scan-i] {
					s++
				}
				if 2*s-i > 2*best-lenB {
					best, lenB = s, i
				}
			}
		}
		// Where they overlap, split at the point that keeps the most
		// matching bytes.
		if lastScan+lenF > scan-lenB {
			overlap := lastScan + lenF - (scan - lenB)
			s, best, split := 0, 0, 0
			for i := 0; i < overlap; i++ {
				if new[lastScan+lenF-overlap+i] == old[lastPos+lenF-overlap+i] {
					s++
				}
				if new[scan-lenB+i] == old[pos-lenB+i] {
					s--
				}
				if s > best {
					best, split = s, i+1
				}
			}
			lenF += split - overlap
			lenB -= split
		}

		for i := 0; i < lenF; i++ {
			dif.WriteByte(new[lastScan+i] - old[lastPos+i])
		}
		extraLen := scan - lenB - (lastScan + lenF)
		extra.Write(new[lastScan+lenF : lastScan+lenF+extraLen])
		emit(lenF, extraLen, pos-lenB-(lastPos+lenF))

		lastScan = scan - lenB
		lastPos = pos - lenB
		lastOffset = pos - scan
	}

	out := bytes.NewBuffer(make([]byte, patchHeader))
	var sizes [3]int
	for i, stream := range []*bytes.Buffer{&ctrl, &dif, &extra} {
		start := out.Len()
		zw, _ := flate.NewWriter(out, flate.BestCompression)
		if _, err := zw.Write(stream.Bytes()); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		sizes[i] = out.Len() - start
	}
	b := out.Bytes()
	copy(b, patchMagic)
	binary.LittleEndian.PutUint64(b[8:], uint64(len(old)))
	binary.LittleEndian.PutUint64(b[16:], uint64(len(new)))
	oldSum, newSum := sha256.Sum256(old), sha256.Sum256(new)
	copy(b[24:], oldSum[:])
	copy(b[56:], newSum[:])
	for i, n := range sizes {
		binary.LittleEndian.PutUint64(b[88+8*i:], uint64(n))
	}
	return b, nil
}

// longestMatch returns the position in old and the length of the longest
// prefix of target that occurs in old. The longest common prefix with
// target is shared by one of the two suffixes around target's insertion
// point in the suffix array.
func longestMatch(sa []int32, old, target []byte) (pos, n int) {
	if len(sa) == 0 || len(target) == 0 {
		return 0, 0
	}
	i := sort.Search(len(sa), func(i int) bool {
		return bytes.Compare(old[sa[i]:], target) >= 0
	})
	for _, j := range []int{i - 1, i} {
		if j < 0 || j >= len(sa) {
			continue
		}
		if l := commonPrefix(old[sa[j]:], target); l > n {
			pos, n = int(sa[j]), l
		}
	}
	return pos, n
}

func commonPrefix(a, b []byte) int {
	n := min(len(a), len(b))
	i := 0
	for i+8 <= n && binary.LittleEndian.Uint64(a[i:]) == binary.LittleEndian.Uint64(b[i:]) {
		i += 8
	}
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// Patch applies a patch made by Diff to old and returns the new file,
// whose SHA-256 digest it checks against the one recorded in the patch.
// It returns ErrPatchBase when old is not the file the patch was made
// from. The new file may be up to 2 GiB; for patches from an untrusted
// source use PatchMax with the size the new file is expected to have.
func Patch(old, patch []byte) ([]byte, error) {
	return PatchMax(old, patch, maxPatchOutput)
}

// PatchMax is Patch for a new file of at most maxSize bytes: a patch that
// claims a larger output is refused before anything is allocated for it.
// A malformed patch is an error, never a panic, and memory use stays
// within about twice the new file's size plus the patch.
func PatchMax(old, patch []byte, maxSize int64) ([]byte, error) {
	if len(patch) < patchHeader || string(patch[:8]) != patchMagic {
		return nil, errors.New("updater: not a patch")
	}
	oldSize := binary.LittleEndian.Uint64(patch[8:])
	newSize := binary.LittleEndian.Uint64(patch[16:])
	if oldSize != uint64(len(old)) {
		return nil, ErrPatchBase
	}
	if sum := sha256.Sum256(old); !bytes.Equal(sum[:], patch[24:56]) {
		return nil, ErrPatchBase
	}
	if maxSize < 0 || newSize > uint64(min(maxSize, maxPatchOutput)) {
		return nil, fmt.Errorf("updater: patch output too large (%d bytes)", newSize)
	}
	size := int64(newSize)
	if int64(int(size)) != size {
		return nil, errors.New("updater: patch output too large for this platform")
	}
	bad := errors.New("updater: corrupt patch")
	rest := patch[patchHeader:]
	var raw [3][]byte
	for i := range raw {
		n := binary.LittleEndian.Uint64(patch[88+8*i:])
		if n > uint64(len(rest)) {
			return nil, errors.New("updater: truncated patch")
		}
		raw[i], rest = rest[:n], rest[n:]
	}
	// The diff and extra streams hold one byte per byte of output between
	// them, so neither may exceed the output; the control stream is read
	// as it is used.
	var streams [2][]byte
	for i := range streams {
		b, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(raw[i+1])), size+1))
		if err != nil {
			return nil, fmt.Errorf("updater: corrupt patch: %w", err)
		}
		if int64(len(b)) > size {
			return nil, bad
		}
		streams[i] = b
	}
	dif, extra := streams[0], streams[1]
	if int64(len(dif))+int64(len(extra)) != size {
		return nil, bad
	}
	ctrl := bufio.NewReader(flate.NewReader(bytes.NewReader(raw[0])))
	// Every triple but a few produces output; bound the rest so a patch
	// of empty triples cannot spin for long.
	triples := size + 1<<20

	out := make([]byte, size)
	oldLen := int64(len(old))
	// Invariants: 0 <= newPos <= size and 0 <= oldPos <= oldLen, so no sum
	// below can overflow.
	var oldPos, newPos int64
	for {
		var t [3]int64
		v, err := binary.ReadVarint(ctrl)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, bad
		}
		t[0] = v
		for i := 1; i < len(t); i++ {
			if t[i], err = binary.ReadVarint(ctrl); err != nil {
				return nil, bad
			}
		}
		if triples--; triples < 0 {
			return nil, bad
		}
		add, cp, seek := t[0], t[1], t[2]
		if add < 0 || cp < 0 || add > int64(len(dif)) || add > size-newPos || add > oldLen-oldPos {
			return nil, bad
		}
		for i := range add {
			out[newPos+i] = dif[i] + old[oldPos+i]
		}
		dif = dif[add:]
		newPos += add
		oldPos += add
		if cp > int64(len(extra)) || cp > size-newPos {
			return nil, bad
		}
		copy(out[newPos:], extra[:cp])
		extra = extra[cp:]
		newPos += cp
		// Diff never seeks outside old.
		if seek < -oldPos || seek > oldLen-oldPos {
			return nil, bad
		}
		oldPos += seek
	}
	if newPos != size {
		return nil, bad
	}
	if sum := sha256.Sum256(out); !bytes.Equal(sum[:], patch[56:88]) {
		return nil, errors.New("updater: patched file has the wrong sha256")
	}
	return out, nil
}
