#!/usr/bin/env bash
set -euo pipefail

arch=${1:?usage: build.sh x64|arm64 output-dir}
out=${2:?usage: build.sh x64|arm64 output-dir}
src=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)

case "$arch" in
    x64) default=x86_64-w64-mingw32-g++ ;;
    arm64) default=aarch64-w64-mingw32-clang++ ;;
    *) echo "Unsupported bridge architecture: $arch" >&2; exit 2 ;;
esac

compiler=("${TANGO_IME_CXX:-$default}")
flags=(-static -static-libgcc -static-libstdc++)
if [[ "$arch" == arm64 && -z "${TANGO_IME_CXX:-}" ]] && ! command -v "$default" >/dev/null; then
    compiler=(zig c++ -target aarch64-windows-gnu)
    flags=(-Wno-macro-redefined -Wno-dll-attribute-on-redeclaration)
fi

"${compiler[@]}" -std=c++17 -O2 -DUNICODE -D_UNICODE -D_WIN32_WINNT=0x0A00 \
    -shared "${flags[@]}" -o "$out/TangoIMEBridge-$arch.dll" \
    "$src"/{tip,langbar,pipe,config,register}.cpp -lole32 -loleaut32 -luuid -ladvapi32 -luser32 -lgdi32
