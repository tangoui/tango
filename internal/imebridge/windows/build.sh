#!/usr/bin/env bash
set -euo pipefail

arch=${1:?usage: build.sh x64|arm64|arm64x output-dir}
out=${2:?usage: build.sh x64|arm64|arm64x output-dir}
src=$(cd "$(dirname "$0")" && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)

case "$arch" in
    x64) default=x86_64-w64-mingw32-g++ ;;
    arm64) default=aarch64-w64-mingw32-clang++ ;;
    arm64x)
        tmp=$(mktemp -d "$out/.arm64x.XXXXXX")
        trap 'rm -rf -- "$tmp"' EXIT
        clang=${TANGO_IME_CLANG:-clang}
        linker=${TANGO_IME_LINK:-lld-link}
        for target in arm64 arm64ec; do
            "$clang" -std=c++17 --target="$target-pc-windows-msvc" -c "$src/arm64x_loadconfig.cpp" -o "$tmp/$target.obj"
        done
        for target in arm64 x64; do
            {
                printf '%s\n' EXPORTS
                for name in DllGetClassObject DllCanUnloadNow DllRegisterServer DllUnregisterServer; do
                    printf '  %s = TangoIMEBridge-%s.%s\n' "$name" "$target" "$name"
                done
            } > "$tmp/$target.def"
            "$linker" /lib "/machine:$target" "/def:$tmp/$target.def" "/out:$tmp/$target.lib"
        done
        "$linker" /dll /noentry /machine:arm64x /noimplib \
            "/defArm64Native:$tmp/arm64.def" "/def:$tmp/x64.def" "$tmp/arm64.obj" "$tmp/arm64ec.obj" \
            "/out:$out/TangoIMEBridge-arm64x.dll" "$tmp/arm64.lib" "$tmp/x64.lib"
        exit
        ;;
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
