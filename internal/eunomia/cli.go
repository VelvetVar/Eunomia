package eunomia

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"golang.org/x/term"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"
)

var Version = "2.4.0"

const help = `EUNOMIA / Native Go homelab manager

Usage:
  eunomia up [--no-animation]       Open the TUI (default command)
  eunomia down                      Shut down the TUI and its SSH sessions
  eunomia list [--json] [--search text]
  eunomia add <name> --host <IP> --user <user> [--port 22] [--description text]
  eunomia edit <name-or-id> [--name text] [--host IP] [--user user]
                           [--port number] [--description text]
  eunomia remove <name-or-id> --yes
  eunomia connect <name-or-id> [--dry-run]
  eunomia export <file.json>        Export the complete Lab to one file
  eunomia import <file.json> --yes  Replace the Lab from an exported file
  eunomia path                      Print the device file location
  eunomia logs [--path]             Show recent diagnostics or their directory
  eunomia doctor [--json]            Check SSH, ping and native terminal support
  eunomia install [--root path] [--bin path] [--no-path]
  eunomia --help | --version

Lab: a add / e edit / Delete remove / f forget fingerprint / d Discover
     arrows or j,k select / Enter SSH / p ping / r reload / m motion / q quit
     Shift+M Move menu / Alt+Up,Down move / Shift+F new folder
     Shift+F on a folder creates a subfolder / Ctrl+F top-level folder
     v details / ? all keys / e rename selected folder
     Move past an expanded folder to join it; collapsed folders are skipped
     Right on a device: move into a folder or back to Lab
     Enter or Space toggles folders / Left collapse / Right expand
Tabs: Ctrl+B, release, then n/p tabs, h or 0 Lab, d Discover, x close
      Ctrl+B then 1-9 selects SSH tabs; Ctrl+B then ? shows all keys
      F6 / Shift+F6 switch; Ctrl+B then b sends Ctrl+B
SSH: arrows / PgUp,PgDn / mouse wheel scroll; Esc returns to live output
     Alt+Up/Down sends shell arrows; full-screen apps keep normal keys
     F7 toggles remote selection mode for prompts; wheel still scrolls
     Right-click copies selected text; otherwise pastes on Windows
     Left-drag selects SSH output; release to copy; Esc clears selection
Forms: Tab/Down next field / Shift+Tab/Up previous field
       Left/Right cursor / Enter save / Esc cancel / Ctrl+U clear
Keys are case-sensitive: Shift+M moves; m toggles animation.
Shift+F creates a folder; f forgets a fingerprint.

Devices, nested folders, order and collapsed state all persist in devices.json.
Older devices.json and lab.json data is migrated automatically.
Passwords and terminal output are never saved. EUNOMIA_HOME overrides storage.
`

type arguments struct {
	Positional []string
	Options    map[string]string
}

func parseArgs(args []string) (arguments, error) {
	result := arguments{Options: map[string]string{}}
	booleans := map[string]bool{"json": true, "yes": true, "dry-run": true, "no-animation": true, "help": true, "version": true, "no-path": true, "path": true}
	values := map[string]bool{"host": true, "user": true, "port": true, "description": true, "name": true, "search": true, "root": true, "bin": true}
	for i := 0; i < len(args); i++ {
		token := args[i]
		if token == "-h" {
			token = "--help"
		}
		if token == "-v" {
			token = "--version"
		}
		if !strings.HasPrefix(token, "-") {
			result.Positional = append(result.Positional, token)
			continue
		}
		if !strings.HasPrefix(token, "--") {
			return result, fmt.Errorf("unknown option: %s", token)
		}
		key, value, equal := strings.Cut(token[2:], "=")
		if booleans[key] && !equal {
			result.Options[key] = "true"
		} else if values[key] {
			if !equal {
				i++
				if i >= len(args) || strings.HasPrefix(args[i], "--") {
					return result, fmt.Errorf("--%s requires a value", key)
				}
				value = args[i]
			}
			result.Options[key] = value
		} else {
			return result, fmt.Errorf("unknown option: %s", token)
		}
	}
	return result, nil
}
func Main(args []string, out, errOut io.Writer) int {
	code, err := runCLI(args, out)
	if err != nil {
		if errors.Is(err, errAlreadyRunning) {
			fmt.Fprintln(errOut, errAlreadyRunning)
			// Keep a console opened by a double-click visible long enough to read.
			// Only this duplicate launch waits; the existing instance is untouched.
			time.Sleep(3 * time.Second)
		} else {
			fmt.Fprintln(errOut, "Eunomia:", err)
		}
		if code == 0 {
			code = 1
		}
	}
	return code
}
func runCLI(args []string, out io.Writer) (resultCode int, resultErr error) {
	parsed, err := parseArgs(args)
	if err != nil {
		return 1, err
	}
	o := parsed.Options
	p := parsed.Positional
	if o["help"] != "" {
		fmt.Fprint(out, help)
		return 0, nil
	}
	if o["version"] != "" {
		fmt.Fprintln(out, Version)
		return 0, nil
	}
	command := "up"
	if len(p) > 0 {
		command = p[0]
	}
	allowed := map[string]string{"up": "no-animation", "dashboard": "no-animation", "down": "", "list": "json search", "add": "host user port description", "edit": "name host user port description", "remove": "yes", "connect": "dry-run", "export": "", "import": "yes", "path": "", "doctor": "json", "install": "root bin no-path", "logs": "path"}
	flags, ok := allowed[command]
	if !ok {
		return 1, fmt.Errorf("unknown command %s; use eunomia --help", command)
	}
	for key := range o {
		if !strings.Contains(" "+flags+" ", " "+key+" ") {
			return 1, fmt.Errorf("--%s is not supported by %s", key, command)
		}
	}
	needsFile := command == "export" || command == "import"
	needsRef := command == "add" || command == "edit" || command == "remove" || command == "connect" || needsFile
	limit := 1
	if needsRef {
		limit = 2
	}
	if len(p) > limit {
		return 1, errors.New("too many arguments; quote names with spaces")
	}
	if needsRef && len(p) < 2 {
		if needsFile {
			return 1, fmt.Errorf("%s requires a filename", command)
		}
		return 1, fmt.Errorf("%s requires a device name or ID", command)
	}
	store := Store{ConfigDir()}
	switch command {
	case "export":
		if err := store.Export(p[1]); err != nil {
			return 1, err
		}
		fmt.Fprintln(out, "Exported devices, folders, order and collapsed state to", p[1])
		return 0, nil
	case "import":
		if o["yes"] == "" {
			return 1, errors.New("import replaces the current Lab; use --yes to confirm")
		}
		if err := store.Import(p[1]); err != nil {
			return 1, err
		}
		fmt.Fprintln(out, "Imported the complete Lab. Press r in Lab to reload.")
		return 0, nil
	case "logs":
		if o["path"] != "" {
			fmt.Fprintln(out, filepath.Join(store.Directory, "logs"))
			return 0, nil
		}
		return 0, PrintDiagnostics(store.Directory, out)
	case "path":
		fmt.Fprintln(out, store.Path())
		return 0, nil
	case "install":
		return 0, Install(o["root"], o["bin"], o["no-path"] != "", out)
	case "down":
		stopped, err := StopRunning(store.Directory)
		if err != nil {
			return 1, err
		}
		if stopped {
			fmt.Fprintln(out, "Eunomia stopped. SSH sessions disconnected; profiles are saved.")
		} else {
			fmt.Fprintln(out, "Eunomia is not running.")
		}
		return 0, nil
	case "doctor":
		report := Doctor()
		if o["json"] != "" {
			encoder := json.NewEncoder(out)
			encoder.SetIndent("", "  ")
			encoder.Encode(report)
		} else {
			printReport(out, report)
		}
		if !report.Ready {
			return 1, nil
		}
		return 0, nil
	case "up", "dashboard":
		// Detect the usual duplicate before initializing the terminal or profile.
		// StartControl still handles simultaneous launches under its exclusive lock.
		if state, err := readRunState(store.Directory); err == nil && processAlive(state.PID) {
			return 1, errAlreadyRunning
		}
		defer func() {
			if resultErr != nil {
				if diagnostics, err := NewDiagnostics(store.Directory); err == nil {
					diagnostics.Event("app.startup_error", map[string]any{"version": Version, "error": resultErr.Error()})
				}
			}
		}()
		if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) || os.Getenv("TERM") == "dumb" {
			return 1, errors.New("the TUI needs an interactive terminal; use eunomia list or --help for command-line mode")
		}
		if _, err := Executable("ssh"); err != nil {
			return 1, err
		}
		if err := terminalSupport(); err != nil {
			return 1, err
		}
		screen, err := tcell.NewScreen()
		if err != nil {
			return 1, err
		}
		app, err := NewApp(screen, store, o["no-animation"] == "" && os.Getenv("EUNOMIA_REDUCED_MOTION") != "1")
		if err != nil {
			return 1, err
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(signals)
		go func() {
			select {
			case <-signals:
				app.Stop()
			case <-app.ctx.Done():
			}
		}()
		return 0, app.Run()
	}
	devices, err := store.Read()
	if err != nil {
		return 1, err
	}
	if command == "list" {
		devices = Search(devices, o["search"])
		if o["json"] != "" {
			encoder := json.NewEncoder(out)
			encoder.SetIndent("", "  ")
			return 0, encoder.Encode(devices)
		}
		if len(devices) == 0 {
			fmt.Fprintln(out, "No devices found. Open eunomia up and press a to add one.")
			return 0, nil
		}
		writer := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "NAME\tADDRESS\tUSER\tPORT\tDESCRIPTION")
		for _, d := range devices {
			fmt.Fprintf(writer, "%s\t%s\t%s\t%d\t%s\n", d.Name, d.Host, d.Username, d.Port, d.Description)
		}
		return 0, writer.Flush()
	}
	ref := p[1]
	if command == "remove" {
		if o["yes"] == "" {
			return 1, errors.New("removal requires --yes; use the TUI for interactive confirmation")
		}
		if err := store.Remove(ref); err != nil {
			return 1, err
		}
		fmt.Fprintln(out, "Removed", safe(ref))
		return 0, nil
	}
	d := Device{Name: ref, Port: 22}
	if command != "add" {
		d, err = Find(devices, ref)
		if err != nil {
			return 1, err
		}
	}
	if command == "connect" {
		if o["dry-run"] != "" {
			return 0, json.NewEncoder(out).Encode(struct {
				Executable string   `json:"executable"`
				Args       []string `json:"args"`
			}{"ssh", SSHArgs(d)})
		}
		file, err := Executable("ssh")
		if err != nil {
			return 1, err
		}
		diagnostics, logErr := NewDiagnostics(store.Directory)
		if logErr != nil {
			fmt.Fprintln(os.Stderr, "Logs unavailable:", logErr)
		}
		diagnostics.Started()
		trace, logErr := diagnostics.SSH(d, file)
		if logErr != nil {
			fmt.Fprintln(os.Stderr, "SSH log unavailable:", logErr)
		}
		defer trace.Finish()
		cmd := exec.Command(file, trace.Args(SSHArgs(d))...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = out
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		code := 0
		if err != nil {
			code = -1
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		}
		fields := map[string]any{"code": code}
		if err != nil {
			fields["error"] = err.Error()
		}
		trace.Event("ssh.exit", fields)
		if err != nil && trace != nil {
			fmt.Fprintln(os.Stderr, "SSH failed; run eunomia logs for details.")
		}
		if exit != nil {
			return exit.ExitCode(), nil
		}
		return 0, err
	}
	if command == "edit" && len(o) == 0 {
		return 1, errors.New("provide at least one field to edit")
	}
	for flag, field := range map[string]*string{"name": &d.Name, "host": &d.Host, "user": &d.Username, "description": &d.Description} {
		if value, ok := o[flag]; ok {
			*field = value
		}
	}
	if value, ok := o["port"]; ok {
		port, err := strconv.Atoi(value)
		if err != nil {
			return 1, errors.New("port must be a number")
		}
		d.Port = port
	}
	editRef := ""
	if command == "edit" {
		editRef = ref
	}
	saved, err := store.Save(d, editRef)
	if err != nil {
		return 1, err
	}
	fmt.Fprintln(out, "Saved", saved.Name)
	return 0, nil
}
func printReport(out io.Writer, r Report) {
	fmt.Fprintf(out, "Eunomia %s startup check (%s/%s)\n\n", Version, r.Platform, r.Arch)
	for _, check := range r.Checks {
		fmt.Fprintf(out, "[%s] %s: %s\n", strings.ToUpper(check.Status), check.Label, safe(check.Detail))
		if check.Fix != "" {
			fmt.Fprintln(out, "  Fix:", check.Fix)
		}
	}
}
