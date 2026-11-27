package proto

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

var vectors = []struct {
	name string
	body any
}{
	{"hello", Hello{PID: 905, Exe: "Test.exe", AppContainer: true, Integrity: 0x1000}},
	{"open", SessionOpen(55)},
	{"close", SessionClose(55)},
	{"key", Key{Session: 55, VK: 65, Scan: 30, Flags: KeyDown | KeyRepeat, Mods: ModShift | ModCapsLock, Text: "A", Caret: Rect{-240, 92, 1, 18}}},
	{"test-key", TestKey{Session: 55, VK: 16, Scan: 54, Flags: KeyUp, Caret: Rect{0, 1, 0, 10}}},
	{"focus", Focus{Session: 55, On: true}},
	{"menu-open", MenuOpen(55)},
	{"menu-pick", MenuPick("about")},
	{"mode-pick", ModePick("en")},
	{"key-result", KeyResult{Session: 55, Eaten: true}},
	{"compose", Compose{Session: 55, Text: "你😀", Cursor: 2}},
	{"commit", Commit{Session: 55, Text: "你好"}},
	{"mode", Mode{ID: "zh", Label: "中文", Short: "中", Icon: "icons/zh.ico"}},
	{"menu", Menu{Items: []MenuItem{{ID: "toggle", Title: "切换中/英", Checked: true}, {Separator: true}, {ID: "about", Title: "关于", Disabled: true}}}},
}

func TestProtoGolden(t *testing.T) {
	for _, v := range vectors {
		t.Run(v.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", v.name+".bin"))
			if err != nil {
				t.Fatal(err)
			}
			msg := Message{Seq: 17, Body: v.body}
			got, err := Marshal(msg)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("Marshal = %x, %v; want %x", got, err, want)
			}
			decoded, err := Read(bytes.NewReader(want))
			if err != nil || !reflect.DeepEqual(decoded, msg) {
				t.Fatalf("Read = %+v, %v; want %+v", decoded, err, msg)
			}
		})
	}
}

func TestRejectMalformedFrame(t *testing.T) {
	good, err := Marshal(Message{Seq: 17, Body: Commit{Session: 3, Text: "中"}})
	if err != nil {
		t.Fatal(err)
	}
	tooLong := bytes.Clone(good)
	binary.LittleEndian.PutUint32(tooLong, 64*1024)
	unknown := bytes.Clone(good)
	unknown[4] = 255
	version := bytes.Clone(good)
	version[6] = 1
	surrogate := bytes.Clone(good)
	surrogate[len(surrogate)-2], surrogate[len(surrogate)-1] = 0x00, 0xd8
	for name, bad := range map[string][]byte{
		"truncated": good[:len(good)-1], "oversize": tooLong,
		"unknown": unknown, "version": version, "invalid surrogate": surrogate,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Read(bytes.NewReader(bad)); err == nil {
				t.Fatal("malformed frame accepted")
			}
		})
	}
}

func TestCPPProtocol(t *testing.T) {
	clang, err := exec.LookPath("clang++")
	if err != nil {
		t.Skip("clang++ not available")
	}
	test := filepath.Join(t.TempDir(), "proto_test")
	if out, err := exec.Command(clang, "-std=c++17", "-Wall", "-Wextra", "-Werror",
		"../windows/proto_test.cpp", "-o", test).CombinedOutput(); err != nil {
		t.Fatalf("compile C++ protocol tests: %v\n%s", err, out)
	}
	if out, err := exec.Command(test, "testdata").CombinedOutput(); err != nil {
		t.Fatalf("C++ protocol tests: %v\n%s", err, out)
	}
}
