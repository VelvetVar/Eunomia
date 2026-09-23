# Troubleshooting

[Back to README](../README.md)

Start with:

```sh
eunomia --version
eunomia doctor
eunomia path
```

`doctor` checks whether the local SSH client starts, whether ping and `ssh-keygen` can be found, and whether native terminal support is available. It doesn't test a remote login.

## The eunomia command isn't found

Open a new terminal after setup. An already open terminal may still have the old PATH.

In PowerShell, check which installations are visible:

```powershell
Get-Command eunomia -All
```

Try the default installed executable directly:

```powershell
& "$env:LOCALAPPDATA\Eunomia\bin\eunomia.exe" up
```

On Linux or macOS:

```sh
command -v eunomia
"$HOME/.local/bin/eunomia" up
```

Use your chosen command directory if you installed with `-BinDir` or `--bin`. If the full path works, rerun setup to register it or add that directory to PATH yourself. The `NoPath` / `--no-path` option deliberately skips registration.

If an older installation is found first, update its PATH entry. The Go edition doesn't require `node`; a Node-related error means you're still running an old launcher.

## SSH won't start

Run `eunomia doctor` and check the SSH path it reports. Rerun setup to install missing system prerequisites. On Windows, this may require permission to enable the OpenSSH Client feature. On Linux, it may require sudo and access to package repositories.

If you keep OpenSSH somewhere else, set `EUNOMIA_SSH` to the absolute path of its executable. On Windows, point it at `ssh.exe`, not a `.cmd` or `.bat` wrapper. Set `EUNOMIA_SSH_KEYGEN` as well if its companion tool isn't beside it or on PATH.

To check a saved device outside the TUI:

```sh
eunomia connect "Atlas NAS"
```

If SSH starts but the login fails, check the device's address, port, default user, and SSH server. Authentication errors come from SSH and use the same keys and configuration as a normal terminal connection.

## The TUI won't open or looks broken

Run `eunomia up` directly in an interactive terminal. Redirected output, pipes, and terminals reporting `TERM=dumb` aren't supported for the TUI. Use `eunomia list --json` for scripts.

Resize the window to at least 64 × 24, and use a UTF-8 font and terminal. Windows needs ConPTY support, available from Windows 10 version 1809.

For a static display, run `eunomia up --no-animation`. Set `NO_COLOR` before starting to turn off colors.

## Tab or Ctrl+C is going to the remote machine

That's the intended behavior inside an SSH session. Use `F6` to switch tabs, or press `Ctrl+B`, release it, then press `0` for Lab. `Ctrl+B` followed by `x` closes the current session.

If your terminal captures F6, use `Ctrl+B` followed by `n` or `p` instead. To send Ctrl+B itself to a remote application such as tmux, press `Ctrl+B` twice.

## Ping says “No reply”, but SSH works

ICMP may be blocked by the device or a firewall. Eunomia doesn't use ping status to block SSH connections. Check the address saved in the profile if both ping and SSH fail.

“Ping N/A” means a ping executable wasn't found. `eunomia doctor` shows the lookup result.

## Discover doesn't find a device

Check the selected subnet. Use left/right to choose an interface, or `c` to enter a range, then `r` to scan it. A custom range must be IPv4 and between `/24` and `/32`.

Discovery only checks port 22. If the device uses another SSH port, add it manually with `a` in Lab. A firewall, a different subnet, or a response slower than 900 ms can also keep a device out of the results.

“Open, unverified” means something accepted the connection but didn't return an SSH banner before the probe finished.

## A device's SSH fingerprint changed

If you expected the change, select the device in Lab and press `f`. Review the target and files before confirming, then reconnect and verify the new key. See [Forget a fingerprint](usage.md#forget-a-fingerprint).

If no matching entry is found, check whether your SSH configuration uses a system-wide known-hosts file or an external `KnownHostsCommand`. The menu edits user known-hosts files only. An unsupported `UserKnownHostsFile` path produces an error rather than guessing which file to edit.

## Saved devices are missing

Run `eunomia path`. Different OS users, `EUNOMIA_HOME` values, or Linux `XDG_CONFIG_HOME` values can point to different device files.

Application files and device files live in separate directories. Moving or reinstalling the application doesn't move the device list. To restore a backup, close Eunomia and copy your saved `devices.json` into the configuration directory, keeping a copy of the existing file first.

## A profile file is invalid or locked

Eunomia reports malformed device files instead of replacing them with an empty list. Back up the file before editing it, or restore a known good copy. A valid file uses the version 1 schema; manually removing IDs or timestamps can make it unreadable.

`devices.json.lock` prevents concurrent writers from overwriting each other. A crash during a write may leave it behind. Close Eunomia and any command that might be editing profiles before removing that lock file and retrying.

The installer has its own `install.lock` in the application install directory. Remove it only after checking that no installer is still running.

## “Already running” or down cannot reach the app

Only one TUI runs per configuration directory. Return to its terminal or run `eunomia down` with the same `EUNOMIA_HOME` value used to start it.

If the control connection can't be reached, close the app in its original terminal. Eunomia won't terminate a process based only on a PID from `running.json`. If every Eunomia instance is closed and the runtime record is invalid, remove `running.json` from the directory reported by `eunomia path`, then try again. Leave `devices.json` in place.

## Reporting a bug

Include your OS, terminal, Eunomia version, what you did, and what happened. `eunomia doctor --json` is useful for startup problems. Remove personal paths, addresses, and other details you don't want to share. Don't include passwords, private keys, or the token from `running.json`.
