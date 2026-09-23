# Contributing

[Back to README](README.md)

Bug reports, fixes, and documentation changes are welcome. For a larger change, open an issue first so the behavior and scope can be discussed before you spend time on it.

## Development setup

Install Go 1.26 or newer and the system OpenSSH client. The Windows TUI also needs ConPTY support. The app uses native terminal APIs; Node.js and a C compiler aren't needed for a normal build.

From a source checkout:

```sh
go mod download
go build ./cmd/eunomia
```

Use a separate configuration directory while developing so your usual device list stays out of test runs.

Linux / macOS:

```sh
export EUNOMIA_HOME="$PWD/.test-data/dev"
go run ./cmd/eunomia up
```

PowerShell:

```powershell
$env:EUNOMIA_HOME = Join-Path $PWD '.test-data\dev'
go run ./cmd/eunomia up
```

Set the same variable in another terminal if you want to run `go run ./cmd/eunomia down` against that instance.

## Source layout

| Path | Purpose |
| --- | --- |
| `cmd/eunomia/` | Application entry point |
| `cmd/release/` | Cross-compilation, checksums, and release archives |
| `internal/eunomia/cli.go` | Commands and argument parsing |
| `internal/eunomia/ui.go` | TUI state and keyboard handling |
| `internal/eunomia/render.go` | Screen rendering and the logo |
| `internal/eunomia/session.go` | SSH terminal emulation and session I/O |
| `internal/eunomia/pty_*.go` | Unix PTYs and Windows ConPTY |
| `internal/eunomia/store.go` | Device validation and persistence |
| `internal/eunomia/network.go` | Ping and SSH discovery |
| `internal/eunomia/hostkeys.go` | Known-hosts lookup and fingerprint removal |
| `internal/eunomia/lifecycle.go` | Instance tracking and `eunomia down` |
| `internal/eunomia/install*.go` | Executable installation and PATH registration |
| `setup.sh`, `setup.ps1`, `setup.cmd` | Package setup and prerequisite handling |
| `docs/` | User documentation |

Platform-specific files use build tags. Keep OS-specific process and terminal code there rather than adding platform branches throughout the TUI.

## Checks

```sh
go fmt ./...
go vet ./...
go test ./...
```

The tests cover device files, CLI commands, TUI controls, discovery limits, fingerprint removal, shutdown, and native terminal sessions. Network tests use loopback connections. Fingerprint tests use temporary known-hosts files.

Some tests launch real child processes through a PTY or ConPTY. Run them somewhere those APIs are available. The native fingerprint test needs `ssh-keygen`, and the real TUI test needs an SSH client.

For concurrency changes, also run:

```sh
go test -race ./...
```

The race detector requires a supported platform and a C toolchain. CI is configured to run tests on Windows, Linux, and macOS, with the race detector on Linux. A successful cross-build doesn't establish that the app runs correctly on the target OS; include which systems you actually tested in your pull request.

For UI changes, check the affected view at 64 × 24 and at a larger size. Check keyboard navigation, resizing, and what happens with an SSH session open in the background.

## Building packages

Run from the repository root:

```sh
go run ./cmd/release
```

The builder creates:

```text
dist/
  eunomia-windows.zip
  eunomia-linux.tar.gz
  SHA256SUMS.txt
  bin/
    windows-amd64/
    windows-arm64/
    linux-amd64/
    linux-arm64/
    darwin-amd64/
    darwin-arm64/
```

Each binary directory contains the executable and its SHA-256 file. The two archives include setup scripts, documentation, dependency licenses, and a manifest of file hashes. Builds use `CGO_ENABLED=0`.

The builder only writes local files. The test workflow uploads build artifacts; it doesn't publish a GitHub release. When preparing a release, attach the two archives and `SHA256SUMS.txt`, and include the macOS binaries and their checksums if distributing them too.

The README's download links point to the archives in `downloads/`. To update them, build the packages, then copy `eunomia-windows.zip`, `eunomia-linux.tar.gz`, and `SHA256SUMS.txt` from `dist/` into `downloads/`. Commit all three together. Leave unpacked binaries in `dist/`; they don't need a second copy in Git.

For an installer change, test from an extracted package, including a path with spaces and a second run over an existing installation. Check that the saved device file survives unchanged. Use a temporary configuration directory and the installer's `NoPath` / `--no-path` option while testing.

## Pull requests

Describe the problem, the resulting behavior, and how you checked it. Keep unrelated changes separate. Add tests for changes to persistence, process cleanup, command handling, or networking when they help catch a regression. Update the docs when a command or key binding changes.

Keep these behaviors intact:

- Passwords and session output stay out of persistent storage.
- Existing version 1 device files remain readable, or get an explicit migration path.
- Closing one SSH tab leaves the others running.
- Plain Tab and Ctrl+C still reach the remote session.
- Discovery stays bounded and cancellable.
- Fingerprint removal requires confirmation and keeps host-key verification enabled.
- A bad profile file is reported without being overwritten.

Don't commit device lists, SSH keys, local test data, tool caches, or unpacked builds. The three published files in `downloads/` are intentional. Keep tests focused on data safety, terminal behavior, and regressions. Documentation should describe the current behavior and use runnable examples.
