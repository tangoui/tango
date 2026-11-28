#!/usr/bin/env bash
# Run in a disposable Linux job with package-manager access.
set -euo pipefail

format=${1:?deb, rpm or arch}
out=${2:?evidence directory}
expected_fcitx=${3:-}
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
cd "$root"
exec > >(tee "$out/build.log") 2>&1

export CGO_ENABLED=0 GOFLAGS=-buildvcs=false
go test ./internal/platform/linux ./internal/platform/imesession ./internal/imebridge/proto ./sys/ime
go build -o "$out/tan" ./cmd/tan
if [[ "$format" == arch ]]; then
    "$out/tan" package -C examples/inputmethod -format deb -o "$out"
else
    "$out/tan" package -C examples/inputmethod -format "$format" -o "$out"
fi
rm "$out/tan"
engine=com.tangoui.inputmethod.example
version=$(pkg-config --modversion Fcitx5Core | tee "$out/fcitx5-version.txt")
if [[ -n "$expected_fcitx" ]]; then
    [[ "$version" == "$expected_fcitx".* ]]
fi
if [[ "$format" == deb ]]; then
    depends=$(dpkg-deb -f "$out"/*.deb Depends)
    printf '%s\n' "$depends" | tee "$out/package-depends.txt"
    minor=${version%.*}
    upper=${minor%.*}.$((${minor##*.} + 1))
    grep -Fq "fcitx5 (>= $minor)" <<<"$depends"
    grep -Fq "fcitx5 (<< $upper)" <<<"$depends"
    installer=()
    if [[ "$(id -u)" != 0 ]]; then installer=(sudo); fi
    "${installer[@]}" dpkg -i "$out"/*.deb
    dpkg -L tango-pinyin > "$out/package-files.txt"
elif [[ "$format" == arch ]]; then
    # Repackage the test payload so pacman owns the files during the smoke.
    stage=$(mktemp -d)
    ar p "$out"/*.deb data.tar.gz | tar -xz -C "$stage"
    arch=$(uname -m)
    package="$out/tango-pinyin-0.1.0-1-$arch.pkg.tar.zst"
    printf 'pkgname = tango-pinyin\npkgver = 0.1.0-1\narch = %s\n' "$arch" > "$stage/.PKGINFO"
    tar --zstd -cf "$package" -C "$stage" .PKGINFO usr
    rm -rf "$stage"
    pacman -U --noconfirm "$package"
    pacman -Qlq tango-pinyin > "$out/package-files.txt"
else
    rpm -U "$out"/*.rpm
    rpm -ql tango-pinyin > "$out/package-files.txt"
fi
grep -F "/usr/share/ibus/component/$engine.xml" "$out/package-files.txt"
grep -F "/usr/share/fcitx5/inputmethod/$engine.conf" "$out/package-files.txt"
plugin=$(grep -E "/fcitx5/$engine[.]so$" "$out/package-files.txt")
test -f "$plugin"
failed=0
for check in ibus fcitx fcitx-panel fcitx-shift; do
    framework=${check%%-*}
    expectation=installed
    if [[ "$check" == fcitx-panel ]]; then expectation=panel-preedit; fi
    if [[ "$check" == fcitx-shift ]]; then expectation=shift-alias; fi
    session=(dbus-run-session -- bash "$here/ci-session.sh" "$framework" "$out" "$expectation")
    if [[ "$(id -u)" == 0 ]]; then
        id ime-test >/dev/null 2>&1 || useradd --create-home ime-test
        chown ime-test "$out"
        runuser -u ime-test -- "${session[@]}" || failed=1
    else
        "${session[@]}" || failed=1
    fi
done
if [[ "$format" == deb ]]; then
    "${installer[@]}" dpkg -r tango-pinyin
elif [[ "$format" == arch ]]; then
    pacman -R --noconfirm tango-pinyin
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
