# Development and publishing

[Back to README](../README.md)

The published repository contains application source, build/setup scripts, user documentation, dependency manifests, licenses, and ready-to-install packages. Tests, benchmarks, development reports, saved Lab data, and temporary files remain local and are ignored by Git.

## Build

Install Go 1.26 or newer and OpenSSH. From the repository root:

```sh
go mod download
go vet ./...
go build -trimpath -o dist/eunomia ./cmd/eunomia
```

On Windows, use `-o dist/eunomia.exe`. Run the built executable directly or use the matching launcher in `scripts/`. Normal builds do not require Node.js or a C compiler.

## Source map

| Path | Purpose |
| --- | --- |
| `cmd/eunomia/` | Application entry point |
| `cmd/release/` | Cross-compilation, checksums, and release archives |
| `internal/eunomia/cli.go` | Commands and argument parsing |
| `internal/eunomia/ui.go`, `shortcuts.go`, `render.go` | TUI state, controls, and rendering |
| `internal/eunomia/lab_*.go`, `store.go`, `profile.go` | Lab organization and single-file persistence |
| `internal/eunomia/session.go`, `pty*.go`, `platform*.go` | SSH sessions and native terminal support |
| `internal/eunomia/network.go`, `hostkeys.go` | Discovery, ping, and SSH trust |
| `internal/eunomia/lifecycle.go`, `diagnostics.go` | Instance control and local diagnostics |
| `internal/eunomia/install*.go`, `executables.go` | Installation and prerequisite detection |
| `scripts/` | Launchers and setup scripts |
| `docs/` | User and developer guides |
| `downloads/` | Published archives and archive checksums |

Platform-specific Go files use build tags. Keep OS-specific process and terminal code there.

## Local verification

If local regression tests are available, run `go test ./...`; concurrency changes should also run `go test -race ./...` with a supported C toolchain. Go tests stay alongside their package source but `*_test.go` files are not tracked. A fresh clone therefore has no regression suite. CI checks compilation, vet, script execution, package checksums, and repeated offline installation on the supported platforms.

Use an isolated `EUNOMIA_HOME` inside the ignored `.test-data/` directory for manual checks. Check UI changes at small and large terminal sizes, including with SSH output in the background. Run Windows installers with `-NoPath -SkipSystemChanges -Offline` and Unix installers with `--no-path --skip-system --offline` when testing without system changes.

## Packages

```sh
go run ./cmd/release
```

This writes Windows and Linux archives, archive checksums, and six platform/architecture binaries under `dist/`. The builder uses an explicit file list; tests, saved profiles, reports, and caches are never packaged. Setup scripts are placed at the top level of each extracted archive. Required dependency licenses are included.

Copy only `eunomia-windows.zip`, `eunomia-linux.tar.gz`, and `SHA256SUMS.txt` into `downloads/`. These are the files linked from the README. Keep unpacked binaries in `dist/`.

Before publishing, review `git status`, the staged diff, and archive contents. Never force-add ignored profiles or credentials. All JSON files, profile/export/backup folders, runtime records, logs, keys, test artifacts, and local reports are ignored. Runtime profiles belong in Eunomia's configuration directory, not the repository.

Removing an already-published file in a new commit removes it from the current branch tree; it does not erase older Git history. Do not rewrite shared history as part of ordinary cleanup.
