# Installation

[Back to README](../README.md)

## Requirements

Use a UTF-8 terminal at least 64 columns wide and 24 rows tall. A 110 × 38 window gives the device list and logo more room.

| System | Requirements |
| --- | --- |
| Windows | Windows 10 version 1809 or newer, with ConPTY support |
| Linux | OpenSSH client; `ping` for reachability checks |
| macOS | System SSH and ping tools |

Windows and Linux release packages include x64 and ARM64 binaries. Go is only needed when building from source. An SSH server must be running on any device you want to connect to.

## Windows

Extract `eunomia-windows.zip`. Open PowerShell or CMD in the extracted `eunomia-windows` folder, where `setup.cmd` is located:

```powershell
.\setup.cmd
```

Setup picks the executable for your CPU, verifies its checksum, and checks that SSH can start. If OpenSSH is missing, it attempts to enable the Windows OpenSSH Client feature. That step may show a UAC prompt and needs access to Windows' feature source.

The default command location is:

```text
%LOCALAPPDATA%\Eunomia\bin\eunomia.exe
```

Open a new terminal, then run `eunomia up`.

To choose different directories:

```powershell
.\setup.cmd -InstallRoot "D:\Apps\Eunomia" -BinDir "D:\Tools"
```

To install without changing PATH or installing Windows features:

```powershell
.\setup.cmd -NoPath -SkipSystemChanges -Offline
```

With `-NoPath`, start Eunomia using the full path printed by setup.

## Linux

```sh
tar -xzf eunomia-linux.tar.gz
cd eunomia-linux
sh ./setup.sh
```

Run the script as your normal user. If SSH or ping is missing, setup uses the available package manager and asks for sudo when needed. It supports apt, dnf, yum, pacman, zypper, and apk.

The command is installed at `~/.local/bin/eunomia`, with a versioned copy under `~/.local/share/eunomia/releases/`. Setup adds the command directory to your shell configuration. Open a new terminal and run `eunomia up`.

Custom directories:

```sh
sh ./setup.sh --root "$HOME/apps/eunomia" --bin "$HOME/.local/bin"
```

Offline installation, without editing shell profiles or invoking a package manager:

```sh
sh ./setup.sh --no-path --skip-system --offline
```

Offline setup needs SSH to be installed already. `sha256sum` or `shasum` is needed to verify the packaged executable. Missing ping support is reported by `eunomia doctor`.

## macOS

The release builder produces these executables:

| Mac | Binary |
| --- | --- |
| Intel | `dist/bin/darwin-amd64/eunomia` |
| Apple silicon | `dist/bin/darwin-arm64/eunomia` |

There isn't a separate macOS installation archive. Use the matching executable from the release build, or [build from source](../README.md#build-from-source). From the directory containing the macOS executable:

```sh
chmod +x eunomia
./eunomia install
```

This installs `~/.local/bin/eunomia` and registers that directory in your shell configuration. Open a new terminal and run `eunomia up`. Downloaded binaries are unsigned, so macOS may ask you to allow them before running.

## What setup changes

Setup copies Eunomia into your chosen install directory and command directory. It keeps a versioned copy under `releases/` and skips copying files that already match. Rerunning setup is supported.

On Windows, PATH registration updates your user environment. On Linux and macOS, it appends a PATH entry to the relevant shell profiles. Use `-NoPath` on Windows or `--no-path` on Unix to leave those settings alone.

Both installers accept `EUNOMIA_INSTALL_ROOT` and `EUNOMIA_BIN_DIR` as default destinations. Explicit command-line paths take precedence and must be absolute.

Saved devices live in a separate [configuration directory](usage.md#saved-data). Reinstalling the application doesn't replace them.

## Install directly from a binary

The executable can install itself when system prerequisites are already available:

```sh
eunomia install --root /absolute/install/path --bin /absolute/command/path --no-path
```

Run that command using the path to your downloaded or built executable, such as `./eunomia install` or `.\eunomia.exe install`. The `install` command checks prerequisites but does not invoke a system package manager.

## Upgrade

1. Run `eunomia down` to close the current TUI and its SSH sessions.
2. Extract the new package into a new folder.
3. Run its setup script with the same install paths you used before.
4. Open a new terminal and run `eunomia --version`.

Windows cannot replace an executable while it is in use. If setup reports a locked file, close the old instance and rerun setup.

The Go edition reads the Node edition's version 1 `devices.json` without an import step. Use the same `EUNOMIA_HOME` if you previously set one. On Windows, `eunomia.exe` takes precedence over an old `eunomia.cmd` in the same directory. If the shell still finds an old install elsewhere, check its path using the [troubleshooting guide](troubleshooting.md#the-eunomia-command-isnt-found).

## Checksums

The release builder writes `dist/SHA256SUMS.txt` for the two archives. Each archive also includes `MANIFEST.sha256` for its contents and a checksum beside each binary. Setup checks the selected binary before installing it.

On Linux, with both archives and `SHA256SUMS.txt` in the current directory:

```sh
sha256sum -c SHA256SUMS.txt
```

On Windows, compute an archive's hash and compare it with its entry in `SHA256SUMS.txt`:

```powershell
Get-FileHash .\eunomia-windows.zip -Algorithm SHA256
```
