package proto

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"text/template"
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
	{"candidates", Candidates{Session: 55, Selected: -1, Items: []Candidate{{Text: "你", Comment: "ni"}}}},
	{"reset", Reset(55)},
	{"candidate-pick", CandidatePick{Session: 55, Revision: 7, Index: 2}},
	{"candidate-list", Candidates{Session: 55, Revision: 7, Selected: -1, Items: []Candidate{{Text: "你", Comment: "ni"}}}},
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

func TestReadUnversionedCandidatePick(t *testing.T) {
	data := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(data, headerSize-4+12)
	binary.LittleEndian.PutUint16(data[4:], uint16(TypeCandidatePick))
	binary.LittleEndian.PutUint16(data[6:], Version)
	binary.LittleEndian.PutUint64(data[8:], 17)
	data = binary.LittleEndian.AppendUint64(data, 55)
	data = binary.LittleEndian.AppendUint32(data, 2)
	got, err := Read(bytes.NewReader(data))
	want := Message{Seq: 17, Body: CandidatePick{Session: 55, Index: 2}}
	if err != nil || got != want {
		t.Fatalf("legacy candidate pick = %+v, %v; want %+v", got, err, want)
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

func TestCPPBridgeLogic(t *testing.T) {
	clang, err := exec.LookPath("clang++")
	if err != nil {
		t.Skip("clang++ not available")
	}
	test := filepath.Join(t.TempDir(), "pipe_logic_test")
	if out, err := exec.Command(clang, "-std=c++17", "-Wall", "-Wextra", "-Werror", "-pthread", "../windows/pipe_logic_test.cpp", "-o", test).CombinedOutput(); err != nil {
		t.Fatalf("compile C++ bridge logic: %v\n%s", err, out)
	}
	if out, err := exec.Command(test).CombinedOutput(); err != nil {
		t.Fatalf("C++ bridge logic: %v\n%s", err, out)
	}
}

func TestBridgeDLLBuilds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-based cross-build test")
	}
	if _, err := exec.LookPath("x86_64-w64-mingw32-g++"); err != nil {
		t.Skip("MinGW-w64 cross-compiler not available")
	}
	dir := t.TempDir()
	if out, err := exec.Command("bash", "../windows/build.sh", "x64", dir).CombinedOutput(); err != nil {
		t.Fatalf("cross-build DLL: %v\n%s", err, out)
	}
	dll := filepath.Join(dir, "TangoIMEBridge-x64.dll")
	out, err := exec.Command("x86_64-w64-mingw32-objdump", "-p", dll).CombinedOutput()
	if err != nil {
		t.Fatalf("check DLL: %v\n%s", err, out)
	}
	for _, name := range []string{"DllGetClassObject", "DllCanUnloadNow", "DllRegisterServer", "DllUnregisterServer"} {
		if !strings.Contains(string(out), name) {
			t.Errorf("missing export %s", name)
		}
	}
	for _, name := range []string{"libstdc++-6.dll", "libgcc_s", "libwinpthread-1.dll"} {
		if strings.Contains(strings.ToLower(string(out)), name) {
			t.Errorf("dynamic dependency %s", name)
		}
	}
}

func TestARM64XForwarderBuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-based cross-build test")
	}
	for _, tool := range []string{"clang", "lld-link", "llvm-readobj"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not available", tool)
		}
	}
	dir := t.TempDir()
	if out, err := exec.Command("bash", "../windows/build.sh", "arm64x", dir).CombinedOutput(); err != nil {
		t.Fatalf("cross-build ARM64X: %v\n%s", err, out)
	}
	dll := filepath.Join(dir, "TangoIMEBridge-arm64x.dll")
	out, err := exec.Command("llvm-readobj", "--coff-load-config", "--coff-exports", dll).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect hybrid DLL: %v\n%s", err, out)
	}
	for _, want := range []string{"Format: COFF-ARM64X", "DynamicRelocations ["} {
		if !strings.Contains(string(out), want) {
			t.Errorf("forwarder is missing %q", want)
		}
	}
	for _, arch := range []string{"arm64", "x64"} {
		for _, name := range []string{"DllGetClassObject", "DllCanUnloadNow", "DllRegisterServer", "DllUnregisterServer"} {
			if want := "ForwardedTo: TangoIMEBridge-" + arch + "." + name; !strings.Contains(string(out), want) {
				t.Errorf("forwarder is missing %q", want)
			}
		}
	}
}

func TestFcitx5PipeLogic(t *testing.T) {
	clang, err := exec.LookPath("clang++")
	if err != nil {
		t.Skip("clang++ not available")
	}
	test := filepath.Join(t.TempDir(), "fcitx-pipe")
	if out, err := exec.Command(clang, "-std=c++17", "-Wall", "-Wextra", "-Werror", "-pthread", "../fcitx5/pipe_logic_test.cpp", "-o", test).CombinedOutput(); err != nil {
		t.Fatalf("compile Fcitx5 pipe logic: %v\n%s", err, out)
	}
	if out, err := exec.Command(test).CombinedOutput(); err != nil {
		t.Fatalf("Fcitx5 pipe logic: %v\n%s", err, out)
	}
}

func TestFcitx5ConfTemplates(t *testing.T) {
	data := struct{ Name, Library, LangCode, Icon, Label string }{"Pinyin", "tango-test", "zh_CN", "pinyin", "中"}
	for file, want := range map[string][]string{
		"addon.conf.in":       {"Name=Pinyin", "Library=tango-test", "Category=InputMethod"},
		"inputmethod.conf.in": {"Name=Pinyin", "Addon=tango-test", "LangCode=zh_CN", "Icon=pinyin", "Label=中"},
	} {
		tmpl, err := template.ParseFiles(filepath.Join("../fcitx5", file))
		if err != nil {
			t.Fatal(err)
		}
		var rendered strings.Builder
		if err := tmpl.Execute(&rendered, data); err != nil {
			t.Fatal(err)
		}
		for _, line := range want {
			if !strings.Contains(rendered.String(), "\n"+line+"\n") {
				t.Errorf("%s lacks %s", file, line)
			}
		}
	}
}
