package eunomia

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

const diagnosticLimit = 1 << 20
const retainedSSHLogs = 10

// Diagnostics only accepts lifecycle events and connection metadata. Terminal
// input, paste buffers and terminal output must never be passed to it.
type Diagnostics struct {
	mu        sync.Mutex
	Directory string
	active    map[string]bool
}

func NewDiagnostics(directory string) (*Diagnostics, error) {
	directory = filepath.Join(directory, "logs")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(directory, "eunomia.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	file.Close()
	return &Diagnostics{Directory: directory, active: map[string]bool{}}, nil
}

func (d *Diagnostics) Event(event string, fields map[string]any) {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if fields == nil {
		fields = map[string]any{}
	}
	fields["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["event"] = event
	fields["pid"] = os.Getpid()
	for key, value := range fields {
		if text, ok := value.(string); ok && len(text) > 4096 {
			fields[key] = text[:4096] + " [truncated]"
		}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return
	}
	data = append(data, '\n')
	path := filepath.Join(d.Directory, "eunomia.log")
	if info, err := os.Stat(path); err == nil && info.Size()+int64(len(data)) > diagnosticLimit {
		// Keep the current log and two previous chunks. No handles remain open
		// between writes, allowing rotation on Windows as well as Unix.
		os.Remove(path + ".2")
		os.Rename(path+".1", path+".2")
		if err := os.Rename(path, path+".1"); err != nil {
			return
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return // Logging must not break an SSH session or freeze the UI.
	}
	defer file.Close()
	file.Write(data)
}

func (d *Diagnostics) Started() {
	d.Event("app.start", map[string]any{"version": Version, "os": runtime.GOOS, "arch": runtime.GOARCH, "go": runtime.Version()})
}

type sshDiagnostic struct {
	log  *Diagnostics
	path string
	once sync.Once
}

func (s *sshDiagnostic) Args(args []string) []string {
	if s == nil {
		return args
	}
	// Avoid DEBUG levels: they can dump configuration and proxy commands.
	return append([]string{"-E", s.path, "-o", "LogLevel=VERBOSE"}, args...)
}

func (d *Diagnostics) SSH(device Device, executable string) (*sshDiagnostic, error) {
	if d == nil {
		return nil, nil
	}
	d.mu.Lock()
	file, err := os.CreateTemp(d.Directory, fmt.Sprintf("ssh-%d-%s-*.log", os.Getpid(), time.Now().UTC().Format("20060102T150405")))
	if err != nil {
		d.mu.Unlock()
		return nil, err
	}
	path := file.Name()
	file.Close()
	d.active[path] = true
	d.pruneSSH()
	d.mu.Unlock()
	d.Event("ssh.start", map[string]any{"session": filepath.Base(path), "executable": executable, "host": device.Host, "port": device.Port, "user": device.Username})
	return &sshDiagnostic{log: d, path: path}, nil
}

func (s *sshDiagnostic) Event(event string, fields map[string]any) {
	if s == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["session"] = filepath.Base(s.path)
	s.log.Event(event, fields)
}

func (s *sshDiagnostic) Finish() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.log.mu.Lock()
		defer s.log.mu.Unlock()
		delete(s.log.active, s.path)
		s.log.pruneSSH()
	})
}

func sshLogs(directory string) []os.FileInfo {
	entries, _ := os.ReadDir(directory)
	var files []os.FileInfo
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "ssh-") && strings.HasSuffix(entry.Name(), ".log") && entry.Type().IsRegular() {
			if info, err := entry.Info(); err == nil {
				files = append(files, info)
			}
		}
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].ModTime().Equal(files[j].ModTime()) {
			return files[i].Name() > files[j].Name()
		}
		return files[i].ModTime().After(files[j].ModTime())
	})
	return files
}

// Called with d.mu held. Active sessions retain their own file until exit.
func (d *Diagnostics) pruneSSH() {
	retained := 0
	for _, file := range sshLogs(d.Directory) {
		path := filepath.Join(d.Directory, file.Name())
		if d.active[path] {
			continue
		}
		parts := strings.SplitN(file.Name(), "-", 3)
		if len(parts) == 3 {
			pid, err := strconv.Atoi(parts[1])
			if err == nil && pid > 0 && pid != os.Getpid() && processAlive(pid) {
				continue // Another TUI or CLI connection may still be writing.
			}
		}
		retained++
		if retained > retainedSSHLogs {
			os.Remove(path)
		}
	}
}

func printLogTail(out io.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	start := max(int64(0), info.Size()-64*1024)
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, 64*1024))
	if err != nil {
		return err
	}
	text := string(data)
	if start > 0 {
		_, text, _ = strings.Cut(text, "\n")
	}
	// Log files can contain server-supplied error text. Do not replay terminal
	// escape sequences when the user displays a diagnostic report.
	text = strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && (unicode.IsControl(r) || unicode.Is(unicode.Cf, r)) {
			return -1
		}
		return r
	}, text)
	fmt.Fprintf(out, "\n--- %s ---\n%s\n", filepath.Base(path), text)
	return nil
}

func PrintDiagnostics(directory string, out io.Writer) error {
	directory = filepath.Join(directory, "logs")
	fmt.Fprintln(out, "Logs:", directory)
	appLog := filepath.Join(directory, "eunomia.log")
	if err := printLogTail(out, appLog); err != nil && !os.IsNotExist(err) {
		return err
	}
	files := sshLogs(directory)
	if len(files) == 0 {
		fmt.Fprintln(out, "No SSH logs yet. Open Eunomia and try the connection again.")
		return nil
	}
	return printLogTail(out, filepath.Join(directory, files[0].Name()))
}
