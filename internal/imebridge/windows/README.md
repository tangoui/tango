# Windows TSF bridge

Build the x64 bridge with mingw-w64 and the ARM64 bridge with llvm-mingw or Zig.

```sh
bash internal/imebridge/windows/build.sh x64 ./build/imebridge
bash internal/imebridge/windows/build.sh arm64 ./build/imebridge
```

Each DLL reads a UTF-16LE INI with a BOM from the same directory and with
the same base name. The `[IME]` section contains `clsid`, `profile`, `connection`,
`name`, `host` (the adjacent executable's filename) and `langid` (four hex digits).

The bridge connects to the input method host for its user and Windows login
session. The host starts at login. The bridge's synchronous key deadline is
150 ms. It passes a key through if the host is unavailable or the reply times
out. A received composition or commit runs in an edit session on the TSF thread.

The protocol uses little-endian fields and limits the total frame size to
64 KiB, including the length prefix. The length counts the bytes following
it. Type and version are 16-bit fields, followed by the 64-bit sequence number.
Strings are UTF-16LE with a 16-bit code-unit count. Composition cursors count
runes. Sequence zero is reserved for unsolicited updates. Edits caused by a
synchronous key carry that key's sequence and precede the key result.

Run the shared codec vectors and timeout checks on the build host.

```sh
CGO_ENABLED=0 go test ./internal/imebridge/proto
```
