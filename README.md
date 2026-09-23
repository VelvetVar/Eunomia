# Eunomia
<img width="800" height="482" alt="ezgif-51f2aadb11d73b21" src="https://github.com/user-attachments/assets/2c3ec917-5135-42dd-9db3-c09ad2d38584" />

A terminal app for keeping track of your homelab and connecting to it over SSH.

Save a device once, give it a name, and open it from the list. Keep several SSH sessions running and switch between them without leaving the terminal. Eunomia is written in Go and runs on Windows, Linux, and macOS.

| Download | Includes |
| --- | --- |
| [Windows ZIP](https://github.com/VelvetVar/Eunomia/raw/refs/heads/main/downloads/eunomia-windows.zip) | EXE files for x64 and ARM64, plus setup |
| [Linux TAR.GZ](https://github.com/VelvetVar/Eunomia/raw/refs/heads/main/downloads/eunomia-linux.tar.gz) | Binaries for x64 and ARM64, plus setup |

[SHA-256 checksums](https://github.com/VelvetVar/Eunomia/blob/main/downloads/SHA256SUMS.txt)

## What it does

- Saves device names, addresses, SSH users, ports, and notes.
- Keeps SSH sessions in separate tabs, each with its own scrollback.
- Pings saved devices about once a minute.
- Finds SSH servers on your local network through the Discover tab.
- Lets you remove a device's saved SSH fingerprint from the menu.
- Shows a slowly rotating orbital logo. Press `m` to pause it.

Passwords are handled by your system's SSH client and are never saved by Eunomia. Your SSH keys, agent, and SSH configuration still work as usual.

## Install

The Windows and Linux packages each include x64 and ARM64 executables. You don't need Go, Node.js, or npm to run them. OpenSSH is required; setup can install it if it's missing.

### Windows

Extract `eunomia-windows.zip`, open a terminal in the extracted `eunomia-windows` folder, and run:

```powershell
.\setup.cmd
```

### Linux

```sh
tar -xzf eunomia-linux.tar.gz
cd eunomia-linux
sh ./setup.sh
```

Run setup as your normal user. It asks for administrator access only when needed to install system tools.

Open a new terminal after setup, then start Eunomia:

```sh
eunomia up
```

For macOS, offline setup, custom install paths, and upgrades, see [Installation](docs/installation.md). If you have a source checkout instead of a release package, see [Build from source](#build-from-source).

## First connection

1. Press `a` to add a device.
2. Enter a name, address, and default SSH user. Use `Tab` to move between fields. The port defaults to `22`.
3. Press `Enter` to save, then `Enter` again to connect.
4. Press `Ctrl+B`, release both keys, then press `0` to return to your device list. Your SSH session stays open.
5. Open another device and use `F6` to switch tabs.

SSH asks for a password or key passphrase when needed. To close Eunomia and all its connections from another terminal:

```sh
eunomia down
```

`up` runs in the current terminal. It doesn't start a background service. You can also press `q` from the device list to quit.

## Keys you'll use most

| Key | Action |
| --- | --- |
| `↑` / `↓` or `j` / `k` | Select a device |
| `Enter` | Connect, or return to that device's open session |
| `a` / `e` | Add / edit a device |
| `Delete` | Remove a saved device |
| `/` | Search devices |
| `D` | Open Discover |
| `p` | Ping devices now |
| `f` | Forget the selected device's SSH fingerprint |
| `F6` / `Shift+F6` | Next / previous tab |
| `Ctrl+B`, then `0`–`9` | `0` for Lab, `1`–`9` for SSH sessions |
| `Ctrl+B`, then `D` | Open Discover from an SSH session |
| `Ctrl+B`, then `x` | Close the current SSH tab |
| `?` | Show the keyboard guide in Lab |

Inside an SSH session, plain `Tab` and `Ctrl+C` go to the remote program. Use the `Ctrl+B` prefix for Eunomia's controls. The [usage guide](docs/usage.md) covers all keys, discovery, and command-line options.

## Where devices are saved

Run `eunomia path` to see the exact location on your machine.

| OS | Default file |
| --- | --- |
| Windows | `%APPDATA%\Eunomia\devices.json` |
| Linux | `~/.config/eunomia/devices.json`, or `$XDG_CONFIG_HOME/eunomia/devices.json` when set |
| macOS | `~/Library/Application Support/Eunomia/devices.json` |

Set `EUNOMIA_HOME` to use another directory. Back up `devices.json` to keep your saved devices. It contains connection details and descriptions; passwords and terminal output aren't written to it.

The Go version uses the same device file as the earlier Node version, so existing devices carry over.

## Build from source

Requires Go 1.26 or newer and OpenSSH. Run these commands from a checkout of this repository.

Linux / macOS:

```sh
go build -trimpath -o eunomia ./cmd/eunomia
./eunomia up
```

Windows:

```powershell
go build -trimpath -o eunomia.exe ./cmd/eunomia
.\eunomia.exe up
```

Run `go run ./cmd/release` to build the Windows and Linux packages, plus macOS executables, under `dist/`. See [Contributing](CONTRIBUTING.md) for tests and the source layout.

## Documentation

- [Installation](docs/installation.md) — requirements, setup options, and upgrades
- [Usage](docs/usage.md) — TUI controls, CLI commands, discovery, and storage
- [Troubleshooting](docs/troubleshooting.md) — startup, SSH, PATH, and profile errors
- [Contributing](CONTRIBUTING.md) — development and testing

## License

[MIT](LICENSE). Release packages include the licenses for bundled Go dependencies.
