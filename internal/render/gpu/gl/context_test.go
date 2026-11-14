//go:build linux || darwin

package gl

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu/gen"
)

// newTestContext returns a headless context closed at the end of the test.
func newTestContext(t *testing.T) *headless {
	t.Helper()
	ctx, err := newHeadlessContext()
	if err != nil {
		t.Fatalf("newHeadlessContext: %v", err)
	}
	t.Cleanup(ctx.close)
	return ctx
}

func TestHeadlessContext(t *testing.T) {
	ctx := newTestContext(t)
	var (
		version          string
		major, minor     int
		maxTex           int32
		glErr            uint32
		renderer, vendor string
	)
	ctx.thread.do(func() {
		f := ctx.funcs
		version = f.GetString(glVERSION)
		renderer, vendor = f.GetString(glRENDERER), f.GetString(glVENDOR)
		major, minor = f.version()
		f.GetIntegerv(glMAX_TEXTURE_SIZE, &maxTex)
		glErr = f.GetError()
	})
	t.Logf("GL_VERSION %q, GL_RENDERER %q, GL_VENDOR %q, GL_MAX_TEXTURE_SIZE %d", version, renderer, vendor, maxTex)
	sMajor, sMinor, ok := parseVersion(version)
	if !ok {
		t.Fatalf("GL_VERSION %q does not start with a version number", version)
	}
	if sMajor != major || sMinor != minor {
		t.Errorf("GL_VERSION %q but GL_MAJOR_VERSION/GL_MINOR_VERSION %d.%d", version, major, minor)
	}
	if major < 3 || major == 3 && minor < 3 {
		t.Errorf("GL %d.%d, want at least 3.3", major, minor)
	}
	if maxTex < 4096 {
		t.Errorf("GL_MAX_TEXTURE_SIZE %d, want at least 4096", maxTex)
	}
	if glErr != glNO_ERROR {
		t.Errorf("glGetError %#x", glErr)
	}
}

// TestHeadlessContexts creates two contexts at once and closes them in
// creation order: each owns its own thread and function table.
func TestHeadlessContexts(t *testing.T) {
	a := newTestContext(t)
	b := newTestContext(t)
	if a.funcs == b.funcs {
		t.Fatal("two contexts share a function table")
	}
	a.close()
	var maxTex int32
	b.thread.do(func() { b.funcs.GetIntegerv(glMAX_TEXTURE_SIZE, &maxTex) })
	if maxTex < 4096 {
		t.Fatalf("second context after closing the first: GL_MAX_TEXTURE_SIZE %d", maxTex)
	}
}

func TestParseVersion(t *testing.T) {
	for s, want := range map[string][2]int{
		"4.5 (Core Profile) Mesa 22.3.6": {4, 5},
		"4.1 Metal - 88":                 {4, 1},
		"3.3.0 NVIDIA 535.54":            {3, 3},
		"OpenGL ES 3.2 Mesa 22.3.6":      {3, 2},
	} {
		major, minor, ok := parseVersion(s)
		if !ok || [2]int{major, minor} != want {
			t.Errorf("parseVersion(%q) = %d, %d, %v; want %v", s, major, minor, ok, want)
		}
	}
	for _, s := range []string{"", "Mesa", "4", "x.y"} {
		if _, _, ok := parseVersion(s); ok {
			t.Errorf("parseVersion(%q) succeeded", s)
		}
	}
}

// quadSamplers returns the names of the sampler uniforms of the generated
// fragment shader (gen.TextureDesc has no name; the declarations are
// "uniform sampler2D <Name>; // binding N" in binding order).
func quadSamplers(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^uniform sampler2D (\w+);`).FindAllStringSubmatch(gen.QuadGLSLFragment, -1) {
		names = append(names, m[1])
	}
	if len(names) != len(gen.QuadTextures) {
		t.Fatalf("quad.frag declares samplers %v, gen.QuadTextures has %d", names, len(gen.QuadTextures))
	}
	return names
}

func TestCompileQuad(t *testing.T) {
	ctx := newTestContext(t)
	names := quadSamplers(t)
	var (
		prog   uint32
		err    error
		locs   []int32
		block  uint32
		glErr  uint32
		active int32
	)
	ctx.thread.do(func() {
		f := ctx.funcs
		prog, err = compileProgram(f, gen.QuadGLSLVertex, gen.QuadGLSLFragment)
		if err != nil {
			return
		}
		defer f.DeleteProgram(prog)
		for _, n := range names {
			locs = append(locs, f.GetUniformLocation(prog, n))
		}
		block = f.GetUniformBlockIndex(prog, "FrameU_ub")
		f.GetProgramiv(prog, glACTIVE_UNIFORMS, &active)
		glErr = f.GetError()
	})
	if err != nil {
		t.Fatal(err)
	}
	if prog == 0 {
		t.Fatal("program 0")
	}
	for i, n := range names {
		if locs[i] < 0 {
			t.Errorf("sampler %s: location %d", n, locs[i])
		}
	}
	if block == glINVALID_INDEX {
		t.Error("uniform block FrameU_ub not found")
	}
	if glErr != glNO_ERROR {
		t.Errorf("glGetError %#x", glErr)
	}
	t.Logf("%d active uniforms, sampler locations %v", active, locs)
}

func TestCompileErrorIncludesLog(t *testing.T) {
	ctx := newTestContext(t)
	const badFrag = "#version 330 core\nout vec4 o;\nvoid main() { o = no_such_function(1.0); }\n"
	// Both stages compile; the varying's types differ, so linking fails.
	const linkVert = "#version 330 core\nout vec4 v;\nvoid main() { v = vec4(1.0); gl_Position = v; }\n"
	const linkFrag = "#version 330 core\nin vec3 v;\nout vec4 o;\nvoid main() { o = vec4(v, 1.0); }\n"
	for name, c := range map[string]struct {
		vert, frag string
		stage      string
	}{
		"vertex":   {"#version 330 core\nvoid main() { gl_Position = undefined_var; }\n", gen.QuadGLSLFragment, "vertex"},
		"fragment": {gen.QuadGLSLVertex, badFrag, "fragment"},
		"link":     {linkVert, linkFrag, "link"},
	} {
		var err error
		var glErr uint32
		ctx.thread.do(func() {
			var prog uint32
			prog, err = compileProgram(ctx.funcs, c.vert, c.frag)
			if err == nil {
				ctx.funcs.DeleteProgram(prog)
			}
			glErr = ctx.funcs.GetError()
		})
		if err == nil {
			t.Errorf("%s: no error", name)
			continue
		}
		msg := err.Error()
		head, log, ok := strings.Cut(msg, "\n")
		if !ok || !strings.Contains(head, c.stage) {
			t.Errorf("%s: error %q does not name the stage on its first line", name, msg)
		}
		// The driver's info log follows (Mesa "0:3(19): error: ...",
		// Apple "ERROR: 0:3: ..."); the first line itself never says
		// "error".
		if strings.Contains(strings.ToLower(head), "error") || !strings.Contains(strings.ToLower(log), "error") {
			t.Errorf("%s: error %q does not carry the driver's info log", name, msg)
		}
		if glErr != glNO_ERROR {
			t.Errorf("%s: glGetError %#x after the failure", name, glErr)
		}
		t.Logf("%s: %s", name, msg)
	}
}

func TestForceCompileSource(t *testing.T) {
	ctx := newTestContext(t)
	forceCompileSource = func(vert, frag string) (string, string) {
		return vert, "#version 330 core\nvoid main() { broken }\n"
	}
	t.Cleanup(func() { forceCompileSource = nil })
	var err error
	ctx.thread.do(func() {
		_, err = compileProgram(ctx.funcs, gen.QuadGLSLVertex, gen.QuadGLSLFragment)
	})
	if err == nil || !strings.Contains(err.Error(), "fragment") {
		t.Fatalf("forced source: %v, want a fragment shader error", err)
	}
}

func TestLoadMissingFunction(t *testing.T) {
	var f glFuncs
	err := f.load(func(name string) uintptr {
		if name == "glTexSubImage2D" || name == "glBlitFramebuffer" {
			return 0
		}
		return 1 // never called
	})
	if err == nil {
		t.Fatal("no error with missing functions")
	}
	msg := err.Error()
	if !strings.Contains(msg, "glTexSubImage2D") || !strings.Contains(msg, "glBlitFramebuffer") {
		t.Errorf("error %q does not name the missing functions", msg)
	}
	if strings.Contains(msg, "glViewport") {
		t.Errorf("error %q names a function that was found", msg)
	}
	if f.Viewport != nil {
		t.Error("functions were bound although the table is incomplete")
	}
	// Every field is looked up by its GL name.
	var asked []string
	if err := new(glFuncs).load(func(name string) uintptr { asked = append(asked, name); return 0 }); err == nil {
		t.Fatal("no error when nothing is found")
	}
	for _, n := range asked {
		if !strings.HasPrefix(n, "gl") || len(n) < 4 {
			t.Errorf("looked up %q", n)
		}
	}
	if len(asked) < 40 {
		t.Errorf("only %d functions looked up", len(asked))
	}
}

func TestThread(t *testing.T) {
	th := newGLThread()
	defer th.close()
	n := 0
	for range 100 {
		th.do(func() { n++ })
	}
	if n != 100 {
		t.Fatalf("%d calls ran", n)
	}
	// A panic inside do reaches the caller; the thread keeps working.
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Errorf("recovered %v", r)
			}
		}()
		th.do(func() { panic("boom") })
	}()
	ran := false
	th.do(func() { ran = true })
	if !ran {
		t.Fatal("thread stopped after a panic")
	}
}

// TestThreadGoexit: runtime.Goexit inside do (t.FailNow in a GL call)
// ends the caller's goroutine too, and afterwards the dead thread neither
// hangs do nor close.
func TestThreadGoexit(t *testing.T) {
	th := newGLThread()
	reached := make(chan bool, 1)
	go func() {
		defer func() { reached <- false }() // runs on Goexit as well
		th.do(runtime.Goexit)
		reached <- true // not reached: the caller exits with fn
	}()
	if <-reached {
		t.Fatal("do returned normally after runtime.Goexit in fn")
	}
	if th.alive() {
		t.Fatal("thread alive after runtime.Goexit in fn")
	}
	result := make(chan any, 1)
	go func() {
		defer func() { result <- recover() }()
		th.do(func() {})
	}()
	select {
	case r := <-result:
		if r != errThreadGone {
			t.Fatalf("later do: recovered %v, want errThreadGone", r)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("later do hangs")
	}
	closed := make(chan struct{})
	go func() { th.close(); th.close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("close hangs")
	}
}

// TestHeadlessGoexit: a context whose thread died closes without hanging.
func TestHeadlessGoexit(t *testing.T) {
	ctx, err := newHeadlessContext()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx.thread.do(runtime.Goexit)
	}()
	<-done
	closed := make(chan struct{})
	go func() { ctx.close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("close of a context whose thread died hangs")
	}
}
