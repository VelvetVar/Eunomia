//go:build windows

package eunomia

import (
	"context"
	"errors"
	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
	"strings"
	"sync"
)

type windowsTerminal struct {
	pty        *conpty.ConPty
	once       sync.Once
	done       chan struct{}
	code       int
	err        error
	cancelWait context.CancelFunc
}

func terminalSupport() error {
	if !conpty.IsConPtyAvailable() {
		return errors.New("Windows ConPTY requires Windows 10 version 1809 or newer")
	}
	return nil
}
func StartTerminal(file string, args []string, width, height int) (TerminalProcess, error) {
	if err := terminalSupport(); err != nil {
		return nil, err
	}
	line := []string{windows.EscapeArg(file)}
	for _, arg := range args {
		line = append(line, windows.EscapeArg(arg))
	}
	p, err := conpty.Start(strings.Join(line, " "), conpty.ConPtyDimensions(width, height), conpty.ConPtyEnv(terminalEnv()))
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	terminal := &windowsTerminal{pty: p, done: make(chan struct{}), cancelWait: cancel}
	go func() {
		code, err := p.Wait(ctx)
		terminal.code = int(code)
		terminal.err = err
		close(terminal.done)
	}()
	return terminal, nil
}
func (p *windowsTerminal) Read(b []byte) (int, error)  { return p.pty.Read(b) }
func (p *windowsTerminal) Write(b []byte) (int, error) { return p.pty.Write(b) }
func (p *windowsTerminal) Resize(w, h int) error       { return p.pty.Resize(w, h) }
func (p *windowsTerminal) Close() error {
	var err error
	p.once.Do(func() {
		if handle, e := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(p.pty.Pid())); e == nil {
			windows.TerminateProcess(handle, 0)
			windows.CloseHandle(handle)
		}
		// WaitForSingleObject must finish before ConPTY closes the process handle.
		p.cancelWait()
		<-p.done
		err = p.pty.Close()
	})
	return err
}
func (p *windowsTerminal) Wait() (int, error) { <-p.done; return p.code, p.err }
