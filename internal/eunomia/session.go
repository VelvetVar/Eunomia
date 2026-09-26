package eunomia

import (
	"errors"
	"fmt"
	"io"
	"sync"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
)

type Session struct {
	mu            sync.Mutex
	Device        Device
	Term          *vt.Emulator
	Process       TerminalProcess
	Exited        bool
	ExitCode      int
	Failure       string
	Unread        bool
	Offset        int
	RemoteKeys    bool // F7 sends navigation keys to prompts on the main screen.
	CursorVisible bool
	closed        bool
	closeOnce     sync.Once
	inputDone     chan struct{}
	writerDone    chan struct{}
	exitDone      chan struct{}
	diagnostic    *sshDiagnostic
}

func NewSession(d Device, process TerminalProcess, w, h int, notify func(*Session)) *Session {
	return newSession(d, process, w, h, notify, nil)
}
func newSession(d Device, process TerminalProcess, w, h int, notify func(*Session), diagnostic *sshDiagnostic) *Session {
	s := &Session{Device: d, Term: vt.NewEmulator(w, h), Process: process, CursorVisible: true, inputDone: make(chan struct{}), writerDone: make(chan struct{}), exitDone: make(chan struct{}), diagnostic: diagnostic}
	s.Term.SetScrollbackSize(2000)
	s.Term.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { s.CursorVisible = visible }})
	// A stopped SSH client must not block the emulator's synchronous input pipe
	// and, through it, the UI. Bound pending input to about 1 MiB per session.
	input := make(chan []byte, 256)
	failInput := func(err error) {
		s.Term.InputPipe().(io.Closer).Close()
		s.mu.Lock()
		if !s.closed && !s.Exited && s.Failure == "" {
			s.Failure = "SSH input: " + err.Error()
		}
		s.mu.Unlock()
		s.diagnostic.Event("ssh.input_error", map[string]any{"error": err.Error()})
		notify(s)
		go s.Close()
	}
	go func() {
		defer close(s.inputDone)
		defer close(input)
		buffer := make([]byte, 4096)
		for {
			n, err := s.Term.Read(buffer)
			if n > 0 {
				select {
				case input <- append([]byte(nil), buffer[:n]...):
				default:
					failInput(errors.New("buffer full; session disconnected"))
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer close(s.writerDone)
		for data := range input {
			for len(data) > 0 {
				n, err := process.Write(data)
				if err != nil {
					failInput(err)
					return
				}
				if n <= 0 || n > len(data) {
					failInput(io.ErrShortWrite)
					return
				}
				data = data[n:]
			}
		}
	}()
	go func() {
		buffer := make([]byte, 32768)
		for {
			n, err := process.Read(buffer)
			if n > 0 {
				s.mu.Lock()
				if !s.closed {
					s.Term.Write(buffer[:n])
					s.Unread = true
				}
				s.mu.Unlock()
				notify(s)
			}
			if err != nil {
				break
			}
		}
	}()
	go func() {
		defer close(s.exitDone)
		code, err := process.Wait()
		s.mu.Lock()
		requestedClose := s.closed
		if err != nil && code < 0 && !s.closed && s.Failure == "" {
			s.Failure = fmt.Sprintf("SSH process: %v", err)
		}
		if diagnostic != nil && code != 0 && !s.closed && s.Failure == "" {
			s.Failure = fmt.Sprintf("SSH exited %d; run eunomia logs for details", code)
		}
		s.Exited = true
		s.ExitCode = code
		s.mu.Unlock()
		fields := map[string]any{"code": code, "close_requested": requestedClose}
		if err != nil {
			fields["error"] = err.Error()
		}
		s.diagnostic.Event("ssh.exit", fields)
		s.diagnostic.Finish()
		notify(s)
	}()
	return s
}
func OpenSession(d Device, w, h int, notify func(*Session)) (*Session, error) {
	return openSession(d, w, h, notify, nil)
}
func openSession(d Device, w, h int, notify func(*Session), diagnostics *Diagnostics) (*Session, error) {
	file, err := Executable("ssh")
	if err != nil {
		return nil, err
	}
	trace, logErr := diagnostics.SSH(d, file)
	if logErr != nil {
		diagnostics.Event("ssh.log_error", map[string]any{"error": logErr.Error()})
	}
	p, err := StartTerminal(file, trace.Args(SSHArgs(d)), w, h)
	if err != nil {
		trace.Event("ssh.start_error", map[string]any{"error": err.Error()})
		trace.Finish()
		return nil, err
	}
	return newSession(d, p, w, h, notify, trace), nil
}
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.Term.InputPipe().(io.Closer).Close()
		s.mu.Unlock()
		s.diagnostic.Event("ssh.close_requested", nil)
		s.Process.Close()
		// Close the concurrency-safe pipe and join its reader before changing
		// the emulator's closed flag, which is not synchronized by the library.
		<-s.inputDone
		<-s.writerDone
		<-s.exitDone
		s.mu.Lock()
		s.Term.Close()
		s.mu.Unlock()
	})
}
func (s *Session) Resize(w, h int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.Term.Resize(w, h)
		s.Process.Resize(w, h)
	}
}
func (s *Session) Scroll(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Term.IsAltScreen() {
		s.Offset = max(0, min(s.Term.ScrollbackLen(), s.Offset+delta*max(1, s.Term.Height()-2)))
	}
}
func (s *Session) ScrollLines(delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Term.IsAltScreen() {
		s.Offset = max(0, min(s.Term.ScrollbackLen(), s.Offset+delta))
	}
}
func (s *Session) ToggleRemoteKeys() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RemoteKeys = !s.RemoteKeys
	s.Offset = 0
}
func (s *Session) ScrollWheel(event *tcell.EventMouse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	delta, button := 3, uv.MouseWheelUp
	if event.Buttons()&tcell.WheelDown != 0 {
		delta, button = -3, uv.MouseWheelDown
	}
	if !s.Term.IsAltScreen() {
		s.Offset = max(0, min(s.Term.ScrollbackLen(), s.Offset+delta))
	} else if !s.Exited {
		// The tab bar occupies row zero. Only forward wheel events when the
		// remote full-screen application has enabled mouse reporting.
		x, y := event.Position()
		s.Term.SendMouse(uv.MouseWheelEvent{X: x, Y: y - 1, Button: button, Mod: terminalModifiers(event.Modifiers())})
	}
}
func (s *Session) SendLiteral(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Exited && !s.closed {
		s.Offset = 0
		io.WriteString(s.Term.InputPipe(), text)
	}
}
func (s *Session) Paste(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Exited && !s.closed {
		s.Offset = 0
		s.Term.Paste(text)
	}
}
func terminalModifiers(modifiers tcell.ModMask) uv.KeyMod {
	var mod uv.KeyMod
	if modifiers&tcell.ModAlt != 0 {
		mod |= uv.ModAlt
	}
	if modifiers&tcell.ModCtrl != 0 {
		mod |= uv.ModCtrl
	}
	if modifiers&tcell.ModShift != 0 {
		mod |= uv.ModShift
	}
	return mod
}
func terminalKey(event *tcell.EventKey) uv.KeyPressEvent {
	mod := terminalModifiers(event.Modifiers())
	key := event.Key()
	code := event.Rune()
	switch key {
	case tcell.KeyRune:
		mod &^= uv.ModShift
	case tcell.KeyEnter:
		code = uv.KeyEnter
		mod &^= uv.ModCtrl
	case tcell.KeyTAB:
		code = uv.KeyTab
		mod &^= uv.ModCtrl
	case tcell.KeyBacktab:
		code = uv.KeyTab
		mod |= uv.ModShift
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		code = uv.KeyBackspace
		mod &^= uv.ModCtrl
	case tcell.KeyEscape:
		code = uv.KeyEscape
		mod &^= uv.ModCtrl
	case tcell.KeyUp:
		code = uv.KeyUp
	case tcell.KeyDown:
		code = uv.KeyDown
	case tcell.KeyLeft:
		code = uv.KeyLeft
	case tcell.KeyRight:
		code = uv.KeyRight
	case tcell.KeyHome:
		code = uv.KeyHome
	case tcell.KeyEnd:
		code = uv.KeyEnd
	case tcell.KeyPgUp:
		code = uv.KeyPgUp
	case tcell.KeyPgDn:
		code = uv.KeyPgDown
	case tcell.KeyDelete:
		code = uv.KeyDelete
	case tcell.KeyInsert:
		code = uv.KeyInsert
	default:
		if key >= tcell.KeyF1 && key <= tcell.KeyF24 {
			code = uv.KeyF1 + rune(key-tcell.KeyF1)
		} else if key >= tcell.KeyCtrlA && key <= tcell.KeyCtrlZ {
			code = 'a' + rune(key-tcell.KeyCtrlA)
			mod |= uv.ModCtrl
		} else if key >= tcell.KeyCtrlBackslash && key <= tcell.KeyCtrlUnderscore {
			code = '\\' + rune(key-tcell.KeyCtrlBackslash)
			mod |= uv.ModCtrl
		} else if key == tcell.KeyCtrlSpace {
			code = ' '
			mod |= uv.ModCtrl
		}
	}
	return uv.KeyPressEvent{Code: code, Mod: mod}
}
func (s *Session) SendKey(event *tcell.EventKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if !s.Term.IsAltScreen() && event.Modifiers() == tcell.ModNone {
		if event.Key() == tcell.KeyEscape && (s.Offset > 0 || s.Exited) {
			s.Offset = 0
			return
		}
	}
	if !s.Term.IsAltScreen() && (!s.RemoteKeys || s.Exited) && event.Modifiers() == tcell.ModNone {
		delta := 0
		switch event.Key() {
		case tcell.KeyUp:
			delta = 1
		case tcell.KeyDown:
			delta = -1
		case tcell.KeyPgUp:
			delta = max(1, s.Term.Height()-2)
		case tcell.KeyPgDn:
			delta = -max(1, s.Term.Height()-2)
		}
		if delta != 0 {
			s.Offset = max(0, min(s.Term.ScrollbackLen(), s.Offset+delta))
			return
		}
	}
	if !s.Exited {
		s.Offset = 0
		// Alt sends an unmodified navigation key to shells and programs that
		// use the main screen. Full-screen applications keep every modifier.
		if !s.Term.IsAltScreen() && !s.RemoteKeys && event.Modifiers() == tcell.ModAlt {
			switch event.Key() {
			case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn:
				event = tcell.NewEventKey(event.Key(), 0, tcell.ModNone)
			}
		}
		s.sendRemoteKey(event)
	}
}

// vt handles plain keys and application cursor mode, but does not encode
// modified navigation keys. Use their standard xterm sequences here.
// The caller holds s.mu.
func (s *Session) sendRemoteKey(event *tcell.EventKey) {
	mod := event.Modifiers() & (tcell.ModShift | tcell.ModAlt | tcell.ModCtrl)
	if mod != 0 {
		parameter := 1
		if mod&tcell.ModShift != 0 {
			parameter += 1
		}
		if mod&tcell.ModAlt != 0 {
			parameter += 2
		}
		if mod&tcell.ModCtrl != 0 {
			parameter += 4
		}
		code, suffix := 1, byte(0)
		switch event.Key() {
		case tcell.KeyUp:
			suffix = 'A'
		case tcell.KeyDown:
			suffix = 'B'
		case tcell.KeyRight:
			suffix = 'C'
		case tcell.KeyLeft:
			suffix = 'D'
		case tcell.KeyHome:
			suffix = 'H'
		case tcell.KeyEnd:
			suffix = 'F'
		case tcell.KeyPgUp:
			code, suffix = 5, '~'
		case tcell.KeyPgDn:
			code, suffix = 6, '~'
		}
		if suffix != 0 {
			fmt.Fprintf(s.Term.InputPipe(), "\x1b[%d;%d%c", code, parameter, suffix)
			return
		}
	}
	s.Term.SendKey(terminalKey(event))
}
