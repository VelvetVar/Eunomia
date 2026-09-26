package eunomia

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestDiagnosticPrivacyAndSessionExit(t *testing.T) {
	a, _ := uiFixture(t)
	trace, err := a.Diagnostics.SSH(fixtureDevice(), "test-ssh")
	if err != nil {
		t.Fatal(err)
	}
	p := newFakeTerminal()
	s := newSession(fixtureDevice(), p, 80, 24, func(*Session) {}, trace)
	a.Sessions, a.View = []*Session{s}, 1
	typeText(a, "DUMMY-typed-secret")
	press(a, tcell.KeyEnter)
	s.Paste("DUMMY-pasted-secret")
	waitUntil(t, time.Second, func() bool { return strings.Contains(p.inputText(), "DUMMY-pasted-secret") })
	if _, err := p.writer.Write([]byte("DUMMY-private-remote-output")); err != nil {
		t.Fatal(err)
	}
	s.Close()
	path := filepath.Join(a.Diagnostics.Directory, "eunomia.log")
	waitUntil(t, time.Second, func() bool {
		data, _ := os.ReadFile(path)
		return bytes.Contains(data, []byte(`"event":"ssh.exit"`))
	})
	files, _ := os.ReadDir(a.Diagnostics.Directory)
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(a.Diagnostics.Directory, file.Name()))
		if err != nil || bytes.Contains(data, []byte("DUMMY-")) {
			t.Fatal("sensitive input/output in diagnostic file", file.Name(), err)
		}
	}
	data, _ := os.ReadFile(path)
	if !bytes.Contains(data, []byte(`"user":"admin"`)) || !bytes.Contains(data, []byte(`"port":22`)) {
		t.Fatal("connection metadata missing")
	}
	args := trace.Args(SSHArgs(fixtureDevice()))
	if strings.Join(args[:4], " ") != "-E "+trace.path+" -o LogLevel=VERBOSE" {
		t.Fatal("unexpected SSH logging settings", args)
	}
}

func TestDiagnosticRotationAndActiveRetention(t *testing.T) {
	d, err := NewDiagnostics(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 250; i++ {
				d.Event("test", map[string]any{"error": strings.Repeat("x", 5000)})
			}
		}()
	}
	wg.Wait()
	for _, name := range []string{"eunomia.log", "eunomia.log.1", "eunomia.log.2"} {
		data, err := os.ReadFile(filepath.Join(d.Directory, name))
		if err != nil || len(data) > diagnosticLimit {
			t.Fatal("rotation did not bound log size", name, len(data), err)
		}
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			if !json.Valid(line) {
				t.Fatal("concurrent events corrupted JSON")
			}
		}
	}
	active, err := d.SSH(fixtureDevice(), "test-ssh")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < retainedSSHLogs+5; i++ {
		trace, err := d.SSH(fixtureDevice(), "test-ssh")
		if err != nil {
			t.Fatal(err)
		}
		trace.Finish()
	}
	if _, err := os.Stat(active.path); err != nil || len(sshLogs(d.Directory)) != retainedSSHLogs+1 {
		t.Fatal("active log was pruned or retention exceeded", err)
	}
	active.Finish()
	if len(sshLogs(d.Directory)) != retainedSSHLogs {
		t.Fatal("closed logs not pruned")
	}
}

func TestLogsCommandAndUnavailableStorage(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("EUNOMIA_HOME", directory)
	var out, errors bytes.Buffer
	if code := Main([]string{"logs", "--path"}, &out, &errors); code != 0 || strings.TrimSpace(out.String()) != filepath.Join(directory, "logs") {
		t.Fatal(code, out.String(), errors.String())
	}
	out.Reset()
	if code := Main([]string{"logs"}, &out, &errors); code != 0 || !strings.Contains(out.String(), "No SSH logs yet") {
		t.Fatal(code, out.String(), errors.String())
	}
	d, err := NewDiagnostics(directory)
	if err != nil {
		t.Fatal(err)
	}
	d.Started()
	trace, err := d.SSH(fixtureDevice(), "test-ssh")
	if err != nil {
		t.Fatal(err)
	}
	defer trace.Finish()
	os.WriteFile(trace.path, []byte(strings.Repeat("old line\n", 10000)+"Permission denied (publickey,password).\x1b[2J\x07\n"), 0600)
	out.Reset()
	if code := Main([]string{"logs"}, &out, &errors); code != 0 || !strings.Contains(out.String(), "Permission denied") || strings.ContainsAny(out.String(), "\x1b\x07") || out.Len() > 70*1024 {
		t.Fatal("unsafe or unbounded log output", code, out.Len(), errors.String())
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	os.WriteFile(blocked, []byte("file"), 0600)
	if unavailable, err := NewDiagnostics(blocked); err == nil || unavailable != nil {
		t.Fatal("bad log path accepted")
	}
	var unavailable *Diagnostics
	unavailable.Started()
	trace, err = unavailable.SSH(fixtureDevice(), "ssh")
	if err != nil || trace != nil || fmt.Sprint(trace.Args([]string{"-p", "22"})) != "[-p 22]" {
		t.Fatal("logging failure altered SSH fallback")
	}
}
