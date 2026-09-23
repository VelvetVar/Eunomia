#!/bin/sh
eunomia_root=$(CDPATH= cd -P "$(dirname "$0")" && pwd) || exit 1
if [ -x "$eunomia_root/eunomia" ]; then exec "$eunomia_root/eunomia" "$@"; fi
printf '%s\n' 'Build the Go executable first: go build -o eunomia ./cmd/eunomia' 'Or extract a release package and run sh ./setup.sh.' >&2
exit 1
