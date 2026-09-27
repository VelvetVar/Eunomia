# Whole-application review — Eunomia 2.4.0

Reviewed on 2026-09-27, starting from `3c240b8` (2.3.1). The review covered all project-owned Go source, tests, launch/setup scripts, release tooling, and CI. Dependency versions were retained; dependency code was inspected where needed to verify rendering and terminal behavior, not audited in full.

## Scope and decisions

| Area | Files reviewed | Outcome |
| --- | --- | --- |
| Entry points and CLI | `cmd/eunomia/main.go`, `cli.go` | Duplicate launches show `Application is already running`, wait three seconds, then exit with status 1. The original instance remains running. |
| UI and shortcuts | `ui.go`, `shortcuts.go`, `lab_ui.go` | Stop the animation timer outside the active Lab list, reuse the redraw timer, coalesce session notifications without losing another tab's exit, and bound whole-paste buffering. |
| Rendering and selection | `render.go`, `selection.go`, both `clipboard_*.go` files | Replace per-cell screen clearing with `Fill`, render emulator graphemes through `Put`, and read `NO_COLOR` once per SSH redraw. Preserve selection, Unicode, and copy/paste routing. |
| SSH and native processes | `session.go`, `pty.go`, both `pty_*.go` and `platform_*.go` files | Retain bounded input, 2,000-line scrollback, process cleanup, and emulator locking. Clear closed-tab references from the session slice so scrollback can be collected. |
| Storage and Lab organization | `store.go`, `profile.go`, `lab_store.go` | Reconcile all groups in one pass, avoid re-sorting during rendering, save a new device and its folder membership in one transaction, and flush new exports/backups before reporting success. |
| Networking and control | `network.go`, `lifecycle.go` | Retain scan/ping worker limits and deadlines; limit concurrent control requests to 16 and reject excess connections promptly. Prune deleted devices from the reachability cache. |
| SSH trust and diagnostics | `hostkeys.go`, `diagnostics.go`, `executables.go` | Retain the existing validation, bounded log reads/rotation, native tool detection, and explicit host-key reset flow. |
| Installation and distribution | `install.go`, both `install_*.go` files, `setup.ps1`, `setup.sh`, `setup.cmd`, `eunomia.cmd`, `eunomia.sh`, `cmd/release/main.go` | Retain isolated release directories, checksums, offline setup, quoting, and profile preservation. Rebuild and exercise Windows installation. |
| Verification | Every existing `*_test.go`, `.github/workflows/test.yml`, local `.tools/go.ps1` wrapper | Run the complete suite and vet; add targeted regressions and repeatable allocation benchmarks. Keep the existing cross-platform and Linux race-test CI jobs. |

All Go filenames without a directory prefix in the table are in `internal/eunomia/`.

## CPU and allocation measurements

Windows amd64, Ryzen 5 7600X, Go 1.27.1. Numbers below are medians of three 500 ms benchmark runs before/after the changes. The Lab workload has 1,000 devices in 100 expanded folders. Drawing uses a 120×40 simulated terminal; SSH contains colored output and scrollback. These measurements are elapsed time and bytes allocated per operation, **not total application RAM or an end-to-end SSH throughput claim**.

| Operation | Before | After | Bytes allocated before | Bytes allocated after |
| --- | ---: | ---: | ---: | ---: |
| Build Lab rows | 74.670 ms | 0.140 ms | 58,875,619 | 529,184 |
| Normalize Lab layout | 99.796 ms | 0.293 ms | 57,799,816 | 379,049 |
| Draw Lab | 226.984 ms | 4.195 ms | 353,472,373 | 3,319,122 |
| Draw SSH session | 2.165 ms | 0.678 ms | 2,358,112 | 149,613 |

The full Lab redraw allocates about 99.1% fewer bytes; SSH redraw allocates about 93.7% fewer. Timing varied with local machine activity, especially the old Lab implementation. The main causes were repeated full-device sorts for each folder, repeated reconciliation during drawing, and deprecated per-cell conversion in tcell. No global caches or garbage-collector tuning were added.

Reproduce with:

```sh
go test ./internal/eunomia -run '^$' -bench 'Benchmark(Lab|SSH)' -benchmem -benchtime=500ms -count=3
```

A separate five-second native Windows empty-Lab sample, following a two-second startup interval, measured about 0.31% of one CPU core with animation and 0% at the process timer's resolution with animation paused, for both old and new builds. It did **not** establish a total-memory reduction: animated working set was 18.34/53.34 MiB old/new (private bytes 53.30/54.71 MiB), and paused working set was 16.99/17.23 MiB (private bytes 52.07/52.55 MiB). This short sample ran isolated instances concurrently and is insufficient to distinguish steady-state behavior from startup, collection, and OS residency effects. Allocation benchmarks provide the stronger comparative evidence. A long session soak with real SSH workloads remains unmeasured.

## Reliability changes

- Creating a device inside a folder validates the destination and persists both changes under one profile lock and atomic replacement. If the folder disappeared while the form was open, nothing is written and the form stays open.
- Export, migration backup, and runtime-record creation share checked write/flush/close handling. Existing files are never overwritten; incomplete files created by a failed write are removed. Active profiles retain the existing atomic replacement path. This is not a claim of power-loss durability on every filesystem.
- A single pending session wake token covers every tab's latest state. A busy tab can no longer fill a queue and suppress another tab's final exit notification. Change detection also includes input failures.
- Removing a tab clears its backing-array pointer. Deleted devices no longer accumulate in the reachability cache.
- Paste buffers stop at 1 MiB of UTF-8 text and reject the entire paste on overflow. No truncated command is sent. Windows retains its additional native clipboard allocation limit.
- Control requests remain authenticated and deadline-bound. Up to 16 can be in flight; excess connections close immediately. Capacity recovers when idle connections expire.
- Duplicate launch detection runs before terminal/profile initialization, with the exclusive runtime-record check still protecting simultaneous starts. Only the second process waits three seconds and exits.

Devices, nested folders, ordering, and collapsed states remain in the single active `devices.json` profile. Existing migration/import/export and collapsed-folder movement behavior is covered by regression tests.

## References and application to this code

- [Go diagnostics](https://go.dev/doc/diagnostics) and [Go testing benchmarks](https://pkg.go.dev/testing#hdr-Benchmarks): measure expensive paths and allocation behavior before making performance claims. Benchmarks are stored in `lab_benchmark_test.go`.
- [tcell Screen API](https://pkg.go.dev/github.com/gdamore/tcell/v2#Screen): use the full-grapheme `Put` API and bulk `Fill` operation. The implementation of the pinned dependency was checked locally to verify the conversion cost.
- [Go timer reset semantics](https://pkg.go.dev/time#Timer.Reset): the reused timer follows the modern channel-timer guarantees supported by this project's Go 1.26 minimum.
- [Google's Site Reliability Engineering, chapter 21: Handling Overload](https://sre.google/sre-book/handling-overload/): resource limits and early rejection guide the bounded paste buffer and control admission limit. Eunomia remains a small local application; it does not need distributed throttling machinery.
- [Designing Data-Intensive Applications, chapter 7 preview: Transactions](https://www.oreilly.com/library/view/designing-data-intensive-applications/9781491903063/ch07.html): the preview highlights partial updates and concurrent writers as failure cases. That informed the device-plus-membership transaction and regression that deletes the destination before saving. Only the publicly available preview was consulted.

## Verification and limits

- `go test -count=1 -timeout=2m ./...` and `go vet ./...`: passed after the duplicate-launch change.
- Regression coverage includes device/folder atomicity, overloaded control recovery, duplicate-launch text/delay and preservation of the original runtime record, session notification coalescing, released tab references, oversized paste rejection, and export non-overwrite behavior.
- Built Windows, Linux, and macOS binaries for amd64 and arm64. Windows launcher reports 2.4.0. Download archive SHA-256 checksums verified.
- Windows package installed twice offline into paths containing spaces, with PATH/system changes disabled. Doctor passed; saved profile bytes were preserved.
- POSIX launch/setup scripts and PowerShell setup passed syntax checks. Linux/macOS builds were cross-compiled here; their native runtime tests were not run locally.
- Native EXE duplicate-launch smoke check: exact requested message, exit status 1 after approximately 3.04 seconds, original process and runtime record intact.
- Local race testing could not run: enabling CGO reported that `gcc` is missing. The existing Linux CI race job has not been executed for this local change.

The review intentionally keeps established worker limits, the simple single-file profile, and the current native SSH architecture. Broad rewrites without a demonstrated defect or measurable benefit would add risk without establishing better CPU or RAM usage.
