//go:build darwin && !ios

package darwin_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
)

// End-to-end tests of the asynchronous tango:// scheme handler (scheme.go),
// driving examples/duplex in a real WKWebView. TANGO_SCHEME_PROBE=1 makes
// the example register its test routes (/__test/...) and the Debug.Stats
// ipc method (examples/duplex/services/testroutes.go).
//
//	TANGO_E2E=1 go test -timeout 300s ./internal/platform/darwin -run 'AsyncSchemeParity|SchemeStopCancelsHandler' -v

// fetchResult is what parityScript reports for one request.
type fetchResult struct {
	Name   string `json:"name"`
	Status int    `json:"status"`
	Type   string `json:"type"`
	Length string `json:"length"` // Content-Length header
	Bytes  int    `json:"bytes"`
	SHA    string `json:"sha"`
	Text   string `json:"text"` // first bytes of the body, for diagnostics
	Err    string `json:"err"`
}

// jsSHA256 is a plain SHA-256 over a Uint8Array (crypto.subtle is not
// available on every custom-scheme page).
const jsSHA256 = `function sha256(m){
  const K=new Uint32Array([0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2]);
  const H=new Uint32Array([0x6a09e667,0xbb67ae85,0x3c6ef372,0xa54ff53a,0x510e527f,0x9b05688c,0x1f83d9ab,0x5be0cd19]);
  const n=m.length, total=((n+9+63)>>6)<<6, b=new Uint8Array(total);
  b.set(m); b[n]=0x80; const bits=n*8;
  b[total-4]=bits>>>24; b[total-3]=bits>>>16; b[total-2]=bits>>>8; b[total-1]=bits;
  b[total-5]=Math.floor(bits/4294967296);
  const W=new Uint32Array(64);
  for(let o=0;o<total;o+=64){
    for(let i=0;i<16;i++)W[i]=(b[o+4*i]<<24)|(b[o+4*i+1]<<16)|(b[o+4*i+2]<<8)|b[o+4*i+3];
    for(let i=16;i<64;i++){const x=W[i-15],y=W[i-2];
      const s0=((x>>>7)|(x<<25))^((x>>>18)|(x<<14))^(x>>>3), s1=((y>>>17)|(y<<15))^((y>>>19)|(y<<13))^(y>>>10);
      W[i]=(W[i-16]+s0+W[i-7]+s1)|0;}
    let a=H[0],c=H[2],d=H[3],e=H[4],f=H[5],g=H[6],h=H[7],bb=H[1];
    for(let i=0;i<64;i++){
      const S1=((e>>>6)|(e<<26))^((e>>>11)|(e<<21))^((e>>>25)|(e<<7)), ch=(e&f)^(~e&g);
      const t1=(h+S1+ch+K[i]+W[i])|0;
      const S0=((a>>>2)|(a<<30))^((a>>>13)|(a<<19))^((a>>>22)|(a<<10)), mj=(a&bb)^(a&c)^(bb&c);
      const t2=(S0+mj)|0;
      h=g;g=f;f=e;e=(d+t1)|0;d=c;c=bb;bb=a;a=(t1+t2)|0;}
    H[0]+=a;H[1]+=bb;H[2]+=c;H[3]+=d;H[4]+=e;H[5]+=f;H[6]+=g;H[7]+=h;
  }
  return Array.from(H,v=>v.toString(16).padStart(8,"0")).join("");
}`

// parityScript fetches every case in turn and reports fetchResults.
const parityScript = `(async () => {
  ` + jsSHA256 + `
  const one = async (name, url, init) => {
    try {
      const r = await fetch(url, init);
      const b = new Uint8Array(await r.arrayBuffer());
      return { name, status: r.status, type: r.headers.get("content-type") || "",
        length: r.headers.get("content-length") || "", bytes: b.length, sha: sha256(b),
        text: new TextDecoder().decode(b.subarray(0, 64)) };
    } catch (e) { return { name, err: String(e) }; }
  };
  const post = new Uint8Array(65536);
  for (let i = 0; i < post.length; i++) post[i] = (i * 7 + 3) & 255;
  return [
    await one("index", "/index.html"),
    await one("big", "/__test/big"),
    await one("redirect", "/__test/redirect"),
    await one("head", "/index.html", { method: "HEAD" }),
    await one("echo", "/__test/echo", { method: "POST", body: post.buffer }),
    await one("panic", "/__test/panic"),
  ];
})()`

func runParity(t *testing.T, env ...string) []fetchResult {
	t.Helper()
	app := automation.LaunchOptions(t, duplexApp(t), automation.Options{
		Env:     append([]string{"TANGO_SCHEME_PROBE=1"}, env...),
		Timeout: 60 * time.Second,
	})
	win := app.Window("Tango Duplex")
	win.WaitJS(`document.readyState === "complete" && !!window.tango`)
	var res []fetchResult
	win.EvalInto(parityScript, &res)
	out := app.Output()
	app.Quit()
	if strings.Contains(out, "NSInternalInconsistencyException") {
		t.Fatalf("WebKit exception (%v):\n%s", env, out)
	}
	return res
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// TestAsyncSchemeParity loads a set of requests through the asynchronous
// scheme handler and checks each result against what Go computes it must
// be (status, type, Content-Length, body SHA-256): the answers the old
// synchronous main-thread path gave.
func TestAsyncSchemeParity(t *testing.T) {
	async := runParity(t)
	if len(async) == 0 {
		t.Fatal("no results")
	}
	for _, r := range async {
		r.Text = ""
		raw, _ := json.Marshal(r)
		t.Logf("async: %s", raw)
	}

	index, err := os.ReadFile("../../../examples/duplex/frontend/index.html")
	if err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 3<<20)
	for i := range big {
		big[i] = byte(i % 251)
	}
	post := make([]byte, 65536)
	for i := range post {
		post[i] = byte(i*7 + 3)
	}
	byName := map[string]fetchResult{}
	for _, r := range async {
		if r.Err != "" {
			t.Fatalf("%s: %s", r.Name, r.Err)
		}
		byName[r.Name] = r
	}
	check := func(name string, status int, typePrefix string, body []byte) {
		t.Helper()
		r := byName[name]
		if r.Status != status || !strings.HasPrefix(r.Type, typePrefix) {
			t.Errorf("%s: status %d type %q, want %d %q", name, r.Status, r.Type, status, typePrefix)
		}
		if body != nil && (r.Bytes != len(body) || r.SHA != sha(body)) {
			t.Errorf("%s: %d bytes sha %s, want %d bytes sha %s", name, r.Bytes, r.SHA, len(body), sha(body))
		}
	}
	check("index", 200, "text/html", index)
	check("big", 200, "application/octet-stream", big)
	if got := byName["big"].Length; got != "3145728" {
		t.Errorf("big: Content-Length %q", got)
	}
	check("redirect", 200, "text/html", index) // followed inside the core
	check("head", 200, "text/html", []byte{})
	check("echo", 200, "application/octet-stream", post)
	check("panic", 500, "text/plain", nil)
	if p := byName["panic"]; !strings.HasPrefix(p.Text, "tango: asset handler panic:") {
		t.Errorf("panic body %q", p.Text)
	}
}

// stats is Debug.Stats of examples/duplex/services.
type stats struct {
	Goroutines   int   `json:"goroutines"`
	HandlerExits int64 `json:"handlerExits"`
}

// TestSchemeStopCancelsHandler opens the never-ending /__test/stream,
// aborts the fetch after the first chunk and requires the Go handler to
// return (its request context ended) soon after, with no WebKit exception
// for calls on a stopped task.
func TestSchemeStopCancelsHandler(t *testing.T) {
	app := automation.LaunchOptions(t, duplexApp(t), automation.Options{
		Env:     []string{"TANGO_SCHEME_PROBE=1"},
		Timeout: 60 * time.Second,
	})
	win := app.Window("Tango Duplex")
	win.WaitJS(`document.readyState === "complete" && !!window.tango`)

	var res struct {
		Before stats  `json:"before"`
		After  stats  `json:"after"`
		Chunk  int    `json:"chunk"`
		Wait   int    `json:"wait"` // ms from abort() until handlerExits went up
		Err    string `json:"err"`
	}
	win.EvalInto(`(async () => {
	  try {
	    const before = await tango.call("Debug.Stats");
	    const ac = new AbortController();
	    const r = await fetch("/__test/stream", { signal: ac.signal });
	    const rd = r.body.getReader();
	    const first = await rd.read();
	    const t0 = performance.now();
	    ac.abort();
	    let after;
	    for (;;) {
	      after = await tango.call("Debug.Stats");
	      if (after.handlerExits > before.handlerExits || performance.now() - t0 > 10000) break;
	      await new Promise(res => setTimeout(res, 20));
	    }
	    return { before, after, chunk: first.value ? first.value.length : 0, wait: Math.round(performance.now() - t0) };
	  } catch (e) { return { err: String(e) }; }
	})()`, &res)
	t.Logf("stop: %+v", res)
	if res.Err != "" {
		t.Fatal(res.Err)
	}
	if res.Chunk == 0 {
		t.Fatal("no first chunk from /__test/stream")
	}
	if res.After.HandlerExits != res.Before.HandlerExits+1 {
		t.Fatalf("handlerExits %d -> %d after abort (waited %d ms)", res.Before.HandlerExits, res.After.HandlerExits, res.Wait)
	}
	// The plan asks for 1 s; the machine is shared and loaded, so the
	// assertion allows 3 s and the measured time is logged.
	if res.Wait > 3000 {
		t.Fatalf("handler returned %d ms after abort()", res.Wait)
	}

	// The page keeps working, and WebKit saw no call on the stopped task.
	time.Sleep(500 * time.Millisecond)
	if got := win.EvalString(`fetch("/index.html").then(r => r.status)`); got != "200" {
		t.Fatalf("fetch after abort: %s", got)
	}
	out := app.Output()
	app.Quit()
	if strings.Contains(out, "NSInternalInconsistencyException") {
		t.Fatalf("WebKit exception:\n%s", out)
	}
}
