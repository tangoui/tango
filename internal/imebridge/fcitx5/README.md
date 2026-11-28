# Tango Fcitx5 addon

Build the module on a matching Linux distribution and architecture with the
Fcitx5 development libraries. Tango packaging supplies the addon name and
installed configuration path to CMake.

```sh
cmake -S internal/imebridge/fcitx5 -B build/fcitx \
  -DTANGO_FCITX_ADDON=tango-pinyin \
  -DTANGO_FCITX_CONFIG=/usr/share/com.example.inputmethod.pinyin/tango-ime.conf
cmake --build build/fcitx
```

The configuration uses `host`, `connection`, and a relative `socket` path
below `XDG_RUNTIME_DIR`. A worker connects over an authenticated same-UID
Unix socket and attempts to start the configured host once if it is absent.
Key decisions time out in 150 ms. The worker reconnects with backoff after
failure. The Fcitx event loop applies preedit, committed text, candidate
lists, and status/menu changes in receive order.

Run the shared wire tests and the portable connection tests on either a
Mac or a Linux build host.

```sh
CGO_ENABLED=0 go test ./internal/imebridge/proto
```
