# Using Eunomia

[Back to README](../README.md)

## Start and stop

```sh
eunomia up
```

Running `eunomia` without arguments also opens the TUI. `up` keeps it in the current terminal. Press `q` in Lab to quit; Eunomia asks before disconnecting active sessions.

From a second terminal:

```sh
eunomia down
```

This requests shutdown and closes the app's SSH connections. Running it again when Eunomia is stopped is harmless. Only one TUI can run per configuration directory. If you use `EUNOMIA_HOME`, set it to the same directory in both terminals.

To start with the logo paused:

```sh
eunomia up --no-animation
```

## Lab

Lab is your saved device list and occupies tab `0`.

| Key | Action |
| --- | --- |
| `↑` / `↓`, `j` / `k` | Move through devices |
| `a` | Add a device |
| `e` | Edit the selected device |
| `Delete` | Remove the selected device after confirmation |
| `Enter` | Open SSH, or return to the device's existing live tab |
| `/` | Search names, hosts, users, and descriptions |
| `v` | Show full device details |
| `f` | Forget the selected device's saved SSH fingerprint |
| `D` or `d` | Open Discover |
| `p` | Ping saved devices now |
| `r` | Reload the device file |
| `m` | Pause or resume the logo |
| `?` | Show the keyboard guide |
| `Esc` | Cancel or return to the list; clear search |
| `q` / `Ctrl+C` | Quit, with confirmation if sessions are active |

### Device forms

A device needs a name, address, and default SSH user. Addresses can be IPv4, IPv6, or hostnames. The port defaults to `22`; the description is optional. Device names must be unique, ignoring case.

Use `Tab`, `Shift+Tab`, or up/down arrows to change fields. Left/right arrows, Home, End, Backspace, and Delete edit the current field. `Ctrl+U` clears it. Press `Enter` to save or `Esc` to cancel.

Pasting inserts text into the current field. Pasted tabs and line breaks become spaces; they won't move between fields, save a form, or confirm a deletion.

Deleting a profile doesn't edit the remote device or remove its SSH fingerprint.

## SSH tabs

Each SSH tab keeps its connection open while you look at other tabs. Opening the same saved device again returns to its live tab.

For a prefix shortcut, press `Ctrl+B`, release both keys, then press the next key.

| Keys | Action |
| --- | --- |
| `F6` / `Shift+F6` | Next / previous tab |
| `Ctrl+B`, then `n` / `p` | Next / previous tab |
| `Ctrl+B`, then `Tab` / `Shift+Tab` | Next / previous tab |
| `Ctrl+B`, then `0` or `h` | Return to Lab |
| `Ctrl+B`, then `1`–`9` | Select an SSH tab by number |
| `Ctrl+B`, then `D` or `d` | Open Discover |
| `Ctrl+B`, then `x` | Close the current SSH tab |
| `↑` / `↓` | Scroll SSH output one line, without a prefix |
| `Page Up` / `Page Down` | Scroll SSH output one page, without a prefix |
| Mouse wheel | Scroll SSH output three lines per notch |
| `Esc` | Return from scrollback to live output |
| `Alt+↑` / `Alt+↓` | Send plain arrows to the remote shell (command history) |
| `Alt+Page Up` / `Alt+Page Down` | Send plain page keys to the remote program |
| `F7` | Toggle remote selection mode for this SSH tab |
| `Ctrl+B`, then `↑` / `↓` | Scroll terminal history one line |
| `Ctrl+B`, then `Page Up` / `Page Down` | Scroll through terminal history |
| `Ctrl+B`, then `u` | Scroll up |
| `Ctrl+B`, then `b` or `Ctrl+B` | Send Ctrl+B to the remote program |

Use next/previous shortcuts to reach sessions beyond tab 9. Plain Tab, Ctrl+C, left/right arrows, and typed text go to the remote session. Pasting is supported, including bracketed paste when the remote application enables it.

Use `↑` / `↓`, page keys, or the wheel to scroll immediately. Press `Esc` to return to live output. Typing or pasting also returns to live output and sends your input to the remote session. For shell command history, use `Alt+↑` / `Alt+↓`. These send plain arrow keys to the shell. Exited tabs can still be scrolled.

Remote programs using the alternate screen, such as Vim and top, receive normal arrow and page keys, including their modifiers. The wheel is forwarded if the program enables mouse reporting; otherwise it has no effect there. Programs that stay on the main screen can receive arrow and page keys through the Alt shortcuts above.

For an installer or menu that asks you to choose an option with the arrows, press `F7`. The footer shows `SELECT: ↑/↓ remote options`. Arrow and page keys now go to the remote program, with their modifiers intact, while the wheel still scrolls Eunomia's history. After scrolling, an arrow key returns to the live prompt and changes the selection; `Esc` returns to live output without changing it. `Enter` confirms the remote choice as usual. Press `F7` again to restore `SCROLL` mode. Each tab keeps its own mode until it is closed.

Eunomia detects alternate-screen applications automatically. Prompts that stay on the main screen need `F7` because ordinary terminal output does not identify whether a program is waiting for an arrow-key selection. The Alt shortcuts send plain arrows and page keys only in `SCROLL` mode.

The wheel also moves through Lab and Discover lists, three rows per notch. It needs a terminal that reports mouse events. Eunomia requests button and wheel events only, so moving the pointer doesn't trigger redraws. Mouse clicks and dragging aren't forwarded. Forms and confirmation dialogs ignore the wheel.

Tabs keep up to 2,000 scrollback lines in memory. `*` marks new output in a background tab; `!` marks a session that has exited. An exited tab keeps its last screen until you close it.

If an SSH process stops accepting input, other tabs and the menu remain usable. Eunomia buffers up to about 1 MiB of pending input for that session, then disconnects it with an error if the buffer fills.

Eunomia uses your system OpenSSH client. Authentication, SSH configuration, keys, and the SSH agent work through that client. Sessions aren't restored when Eunomia restarts. If you need remote work to survive a disconnect, run it inside a remote session manager such as tmux.

## Discover

Press `D` in Lab, or `Ctrl+B` followed by `D` in an SSH tab.

The first visit scans the selected local private IPv4 range. If no range is available, press `c` and enter one, such as `192.168.1.0/24` or a single IPv4 address.

| Key | Action |
| --- | --- |
| `←` / `→` | Choose another local range |
| `r` | Scan the selected range |
| `x` | Cancel the current scan |
| `c` | Enter a custom IPv4 address or subnet |
| `↑` / `↓`, `j` / `k` | Select a result |
| `Enter` or `a` | Open a save form, or select an existing saved device |
| `Esc` | Return to Lab; cancel subnet entry when editing |
| `q` | Quit Eunomia |

Discovery checks TCP port `22` with up to 24 connections at a time and a 900 ms deadline per address. An SSH banner confirms the service. A port that accepts a connection without returning an SSH banner is marked **Open, unverified**. Discovery doesn't send credentials or attempt to log in.

Custom ranges are limited to `/24` through `/32`. A larger local network is narrowed to the `/24` containing your interface's address. Discovery is IPv4 only; saved IPv6 devices can still be used for SSH.

Results aren't added to your device list until you save them. Scans don't repeat on a timer. Devices on other ports, filtered ports, or slow connections may be missed. The results list listening servers, not other users' active SSH sessions.

## Reachability

Saved devices are pinged on startup and about every 60 seconds. Press `p` in Lab to run a check immediately. Eunomia runs at most four probes at once and gives each probe up to 3.5 seconds.

| Status | Meaning |
| --- | --- |
| Reachable | The device answered a ping |
| No reply | The ping failed or timed out |
| Checking | A check is in progress |
| Ping N/A | A usable ping executable wasn't found |

A firewall can block ping while allowing SSH. A “No reply” result doesn't prevent you from connecting. Ping results aren't saved between runs.

## Forget a fingerprint

After a device is rebuilt or its SSH host key changes:

1. Select it in Lab and press `f`.
2. Check the host and `known_hosts` files shown in the confirmation.
3. Press `y` to remove matching entries, or `n` / `Esc` to cancel.
4. Reconnect and verify the new fingerprint before accepting it.

Eunomia reads the effective SSH configuration with `ssh -G` and uses `ssh-keygen` to find and remove entries. This covers hashed entries, custom ports, IPv6, and `HostKeyAlias`. OpenSSH creates `.old` backups of the edited files.

This action changes user `known_hosts` files. System-wide host-key files and external `KnownHostsCommand` providers aren't edited. If a configured path can't be resolved, Eunomia reports the problem. Removing a fingerprint leaves the saved device and existing connections in place.

## Command-line reference

These commands work outside the full-screen interface:

| Command | Purpose |
| --- | --- |
| `eunomia list` | List saved devices |
| `eunomia list --json` | Print devices as JSON |
| `eunomia list --search storage` | Filter the list |
| `eunomia add <name> --host <host> --user <user>` | Save a device |
| `eunomia edit <name-or-id> <options>` | Change a saved device |
| `eunomia remove <name-or-id> --yes` | Delete a saved device |
| `eunomia connect <name-or-id>` | Start SSH directly in the current terminal |
| `eunomia connect <name-or-id> --dry-run` | Print the SSH command's executable name and argument list as JSON |
| `eunomia path` | Print the device file path |
| `eunomia doctor [--json]` | Check runtime prerequisites |
| `eunomia --version` | Print the version |
| `eunomia --help` | Show command help |

Names containing spaces need quotes. Commands that select a device accept its ID or exact name, ignoring case.

```sh
eunomia add "Atlas NAS" --host 192.168.1.20 --user admin --description "Storage"
eunomia edit "Atlas NAS" --port 2222
eunomia list --search storage
eunomia connect "Atlas NAS"
```

`add` accepts `--host`, `--user`, `--port`, and `--description`. `edit` accepts the same options plus `--name`. Pass `--description=` to clear a description. `remove` requires `--yes`; use Delete in the TUI if you want an interactive confirmation.

See [Installation](installation.md#install-directly-from-a-binary) for `eunomia install` and its options.

## Saved data

Profiles are stored as plain JSON. Use `eunomia path` to find yours.

| OS | Default location |
| --- | --- |
| Windows | `%APPDATA%\Eunomia\devices.json` |
| Linux | `$XDG_CONFIG_HOME/eunomia/devices.json`, falling back to `~/.config/eunomia/devices.json` |
| macOS | `~/Library/Application Support/Eunomia/devices.json` |

Each device stores its ID, name, host, user, port, description, and creation/update times. Passwords and terminal output aren't persisted. The file is not encrypted, so keep secrets out of names and descriptions.

For a backup, stop Eunomia and copy `devices.json`. To move to another machine, stop Eunomia there and place the file in the directory reported by `eunomia path`. Keep a copy of any destination file you want to retain. SSH keys, SSH configuration, and host-key files are managed separately by OpenSSH.

The configuration directory can also contain `running.json`, which identifies the active TUI for `eunomia down`, and a temporary `devices.json.lock` while a writer is active. Don't copy these as part of a backup.

Use `EUNOMIA_HOME` to keep a separate device list. In PowerShell:

```powershell
$env:EUNOMIA_HOME = 'D:\Homelab\Eunomia'
eunomia up
```

On Linux or macOS:

```sh
export EUNOMIA_HOME="$HOME/homelab/eunomia"
eunomia up
```

These examples set the variable for the current shell. Set the same value in any other terminal where you run `eunomia down`, `list`, or other commands for that device list.

## Environment variables

| Variable | Effect |
| --- | --- |
| `EUNOMIA_HOME` | Override the configuration directory |
| `EUNOMIA_SSH` | Absolute path to an OpenSSH executable |
| `EUNOMIA_SSH_KEYGEN` | Absolute path to `ssh-keygen` |
| `EUNOMIA_PING` | Absolute path to `ping` |
| `EUNOMIA_PING6` | Absolute path to `ping6`, used for IPv6 on macOS |
| `EUNOMIA_REDUCED_MOTION=1` | Start with animation disabled |
| `NO_COLOR` | Disable colors when the variable is present |
| `EUNOMIA_INSTALL_ROOT` | Default application install directory |
| `EUNOMIA_BIN_DIR` | Default command directory |

Without executable overrides, Eunomia searches PATH and standard system locations, including Windows OpenSSH and Git, Homebrew, and common Nix paths.
