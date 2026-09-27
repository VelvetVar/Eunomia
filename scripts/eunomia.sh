#!/bin/sh
eunomia_root=$(CDPATH= cd -P "$(dirname "$0")" && pwd) || exit 1
if [ -f "$eunomia_root/../go.mod" ]; then eunomia_root=$(CDPATH= cd -P "$eunomia_root/.." && pwd) || exit 1; fi
if [ -x "$eunomia_root/dist/eunomia" ]; then exec "$eunomia_root/dist/eunomia" "$@"; fi
if [ -x "$eunomia_root/eunomia" ]; then exec "$eunomia_root/eunomia" "$@"; fi
printf '%s\n' 'Build from the repository root: go build -o dist/eunomia ./cmd/eunomia' 'Or extract a release package and run sh ./setup.sh.' >&2
exit 1
