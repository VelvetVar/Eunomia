#!/bin/sh
# Eunomia native binary setup. No language runtime or compiler is needed in a release package.
set -eu
eunomia_root=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    eunomia_ps="$eunomia_root/setup.ps1"
    if command -v cygpath >/dev/null 2>&1; then eunomia_ps=$(cygpath -w "$eunomia_ps"); fi
    exec powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$eunomia_ps" "$@" ;;
  Linux) eunomia_os=linux ;;
  Darwin) eunomia_os=darwin ;;
  *) printf '%s\n' 'Unsupported operating system.' >&2; exit 1 ;;
esac
# Validate options before building anything or invoking a package manager.
eunomia_install=${EUNOMIA_INSTALL_ROOT:-"$HOME/.local/share/eunomia"}
eunomia_bin=${EUNOMIA_BIN_DIR:-"$HOME/.local/bin"}
eunomia_no_path=''
eunomia_skip=0
eunomia_offline=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --root|--bin)
      [ "$#" -ge 2 ] && [ -n "$2" ] || { printf '%s requires a path\n' "$1" >&2; exit 1; }
      if [ "$1" = --root ]; then eunomia_install=$2; else eunomia_bin=$2; fi
      shift 2 ;;
    --no-path) eunomia_no_path=1; shift ;;
    --offline) eunomia_offline=1; eunomia_skip=1; shift ;;
    --skip-system) eunomia_skip=1; shift ;;
    --help|-h) printf '%s\n' 'Usage: sh setup.sh [--root PATH] [--bin PATH] [--no-path] [--offline] [--skip-system]'; exit 0 ;;
    *) printf 'Unknown setup option: %s\n' "$1" >&2; exit 1 ;;
  esac
done
for eunomia_path in "$eunomia_install" "$eunomia_bin"; do
  case "$eunomia_path" in /*) ;; *) printf '%s\n' 'Install and command directories must be absolute paths.' >&2; exit 1 ;; esac
done
case "$(uname -m)" in x86_64|amd64) eunomia_arch=amd64 ;; aarch64|arm64) eunomia_arch=arm64 ;; *) printf '%s\n' 'This release includes x64 and ARM64 binaries. Build from Go source on other architectures.' >&2; exit 1 ;; esac
eunomia_binary="$eunomia_root/bin/$eunomia_os-$eunomia_arch/eunomia"
if [ ! -f "$eunomia_binary" ]; then
  if [ -x "$eunomia_root/eunomia" ]; then eunomia_binary="$eunomia_root/eunomia"
  elif command -v go >/dev/null 2>&1 && [ -f "$eunomia_root/go.mod" ]; then
    if [ "$eunomia_offline" -eq 1 ]; then
      (cd "$eunomia_root" && GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local CGO_ENABLED=0 go build -trimpath -o eunomia ./cmd/eunomia)
    else
      (cd "$eunomia_root" && CGO_ENABLED=0 go build -trimpath -o eunomia ./cmd/eunomia)
    fi
    eunomia_binary="$eunomia_root/eunomia"
  else printf '%s\n' 'Binary missing. Extract a complete Eunomia release package, or install Go 1.26+ to build this source checkout.' >&2; exit 1; fi
fi
if [ -f "$eunomia_binary.sha256" ]; then
  eunomia_expected=$(awk '{print $1}' "$eunomia_binary.sha256")
  if command -v sha256sum >/dev/null 2>&1; then eunomia_actual=$(sha256sum "$eunomia_binary" | awk '{print $1}')
  elif command -v shasum >/dev/null 2>&1; then eunomia_actual=$(shasum -a 256 "$eunomia_binary" | awk '{print $1}')
  else printf '%s\n' 'Install sha256sum or shasum to verify the executable.' >&2; exit 1; fi
  [ "$eunomia_actual" = "$eunomia_expected" ] || { printf '%s\n' 'Executable checksum mismatch. Re-extract the package.' >&2; exit 1; }
fi
chmod u+x "$eunomia_binary"
as_admin() {
  if [ "$(id -u)" -eq 0 ]; then "$@"
  elif command -v sudo >/dev/null 2>&1; then sudo "$@"
  else printf '%s\n' 'Install the OpenSSH client and ping through your administrator, then rerun setup.' >&2; return 1; fi
}
PATH="$PATH:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"; export PATH
eunomia_missing=0
for eunomia_tool in ssh ssh-keygen ping; do command -v "$eunomia_tool" >/dev/null 2>&1 || eunomia_missing=1; done
if [ "$eunomia_missing" -eq 1 ] && [ "$eunomia_skip" -eq 0 ] && [ "$eunomia_os" = linux ]; then
  printf '%s\n' 'Installing missing SSH/ping tools; sudo may ask for your password.'
  if command -v apt-get >/dev/null 2>&1; then as_admin apt-get update && as_admin apt-get install -y openssh-client iputils-ping
  elif command -v dnf >/dev/null 2>&1; then as_admin dnf install -y openssh-clients iputils
  elif command -v yum >/dev/null 2>&1; then as_admin yum install -y openssh-clients iputils
  elif command -v pacman >/dev/null 2>&1; then as_admin pacman -S --needed --noconfirm openssh iputils
  elif command -v zypper >/dev/null 2>&1; then as_admin zypper --non-interactive install openssh-clients iputils
  elif command -v apk >/dev/null 2>&1; then as_admin apk add openssh-client iputils
  else printf '%s\n' 'Install OpenSSH client and ping using your system package manager.' >&2; fi
fi
if [ -n "$eunomia_no_path" ]; then exec "$eunomia_binary" install --root "$eunomia_install" --bin "$eunomia_bin" --no-path; fi
exec "$eunomia_binary" install --root "$eunomia_install" --bin "$eunomia_bin"
