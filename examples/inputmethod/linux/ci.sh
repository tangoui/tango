#!/usr/bin/env bash
# Run in a disposable Linux job with package-manager access.
set -euo pipefail

format=${1:?deb or rpm}
out=${2:?evidence directory}
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
cd "$root"
exec > >(tee "$out/build.log") 2>&1

export CGO_ENABLED=0 GOFLAGS=-buildvcs=false
go test ./internal/platform/linux ./internal/platform/imesession ./internal/imebridge/proto ./sys/ime
go build -o "$out/tan" ./cmd/tan
"$out/tan" package -C examples/inputmethod -format "$format" -o "$out"
rm "$out/tan"
engine=com.tangoui.inputmethod.example
if [[ "$format" == deb ]]; then
    sudo dpkg -i "$out"/*.deb
    dpkg -L tango-pinyin > "$out/package-files.txt"
else
    rpm -U "$out"/*.rpm
    rpm -ql tango-pinyin > "$out/package-files.txt"
fi
grep -F "/usr/share/ibus/component/$engine.xml" "$out/package-files.txt"
grep -F "/usr/share/fcitx5/inputmethod/$engine.conf" "$out/package-files.txt"
plugin=$(grep -E "/fcitx5/$engine[.]so$" "$out/package-files.txt")
test -f "$plugin"
pkg-config --modversion Fcitx5Core | tee "$out/fcitx5-version.txt"
failed=0
for framework in ibus fcitx; do
    session=(dbus-run-session -- bash "$here/ci-session.sh" "$framework" "$out")
    if [[ "$(id -u)" == 0 ]]; then
        id ime-test >/dev/null 2>&1 || useradd --create-home ime-test
        chown ime-test "$out"
        runuser -u ime-test -- "${session[@]}" || failed=1
    else
        "${session[@]}" || failed=1
    fi
done
if [[ "$format" == deb ]]; then
    sudo dpkg -r tango-pinyin
else
    rpm -e tango-pinyin
fi
test ! -e "$plugin"
test ! -e "/usr/share/ibus/component/$engine.xml"
test ! -e "/usr/share/fcitx5/addon/$engine.conf"
test ! -e "/usr/share/fcitx5/inputmethod/$engine.conf"
for framework in ibus fcitx; do
    session=(dbus-run-session -- bash "$here/ci-session.sh" "$framework" "$out" removed)
    if [[ "$(id -u)" == 0 ]]; then
        runuser -u ime-test -- "${session[@]}" || failed=1
    else
        "${session[@]}" || failed=1
    fi
done
echo "Package removal verified" | tee "$out/uninstall.txt"
exit "$failed"
