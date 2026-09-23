//go:build !windows

package eunomia

import (
	"github.com/creack/pty"
	"os"
	"os/exec"
	"sync"
)

type unixTerminal struct {
	file *os.File
	cmd  *exec.Cmd
	done chan struct{}
	err  error
	once sync.Once
}

func terminalSupport() error { return nil }
func StartTerminal(file string, args []string, width, height int) (TerminalProcess, error) {
	cmd := exec.Command(file, args...)
	cmd.Env = terminalEnv()
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
	if err != nil {
		return nil, err
	}
	p := &unixTerminal{file: f, cmd: cmd, done: make(chan struct{})}
	go func() { p.err = cmd.Wait(); close(p.done) }()
	return p, nil
}
func (p *unixTerminal) Read(b []byte) (int, error)  { return p.file.Read(b) }
func (p *unixTerminal) Write(b []byte) (int, error) { return p.file.Write(b) }
func (p *unixTerminal) Resize(w, h int) error {
	return pty.Setsize(p.file, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
}
func (p *unixTerminal) Close() error {
	var err error
	// Use the retained process object, never a potentially recycled process-group
	// ID from an exited tab. Closing the controlling PTY also hangs up its shell.
	p.once.Do(func() { p.cmd.Process.Kill(); err = p.file.Close() })
	return err
}
func (p *unixTerminal) Wait() (int, error) { <-p.done; return p.cmd.ProcessState.ExitCode(), p.err }
