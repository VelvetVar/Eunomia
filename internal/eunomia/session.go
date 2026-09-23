package eunomia

import (
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
	"io"
	"sync"
)

type Session struct {
	mu            sync.Mutex
	Device        Device
	Term          *vt.Emulator
	Process       TerminalProcess
	Exited        bool
	ExitCode      int
	Unread        bool
	Offset        int
	CursorVisible bool
	closed        bool
	notify        func(*Session)
	closeOnce     sync.Once
	writeMu       sync.Mutex
	inputDone     chan struct{}
}

func NewSession(d Device, process TerminalProcess, w, h int, notify func(*Session)) *Session {
	s := &Session{Device: d, Term: vt.NewEmulator(w, h), Process: process, CursorVisible: true, notify: notify, inputDone: make(chan struct{})}
	s.Term.SetScrollbackSize(2000)
	s.Term.SetCallbacks(vt.Callbacks{CursorVisibility: func(visible bool) { s.CursorVisible = visible }})
	// Drain terminal replies and encoded keyboard input independently of screen updates.
	go func() {
		defer close(s.inputDone)
		buffer := make([]byte, 4096)
		for {
			n, err := s.Term.Read(buffer)
			if n > 0 {
				s.writeMu.Lock()
				process.Write(buffer[:n])
				s.writeMu.Unlock()
			}
			if err != nil {
				return
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
		code, _ := process.Wait()
		s.mu.Lock()
		s.Exited = true
		s.ExitCode = code
		s.mu.Unlock()
		notify(s)
	}()
	return s
}
func OpenSession(d Device, w, h int, notify func(*Session)) (*Session, error) {
	file, err := Executable("ssh")
	if err != nil {
		return nil, err
	}
	p, err := StartTerminal(file, SSHArgs(d), w, h)
	if err != nil {
		return nil, err
	}
	return NewSession(d, p, w, h, notify), nil
}
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.Process.Close()
		s.mu.Lock()
		// Close the concurrency-safe pipe and join its reader before changing
		// the emulator's closed flag, which is not synchronized by the library.
		s.Term.InputPipe().(io.Closer).Close()
		<-s.inputDone
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
func terminalKey(event *tcell.EventKey) uv.KeyPressEvent {
	var mod uv.KeyMod
	if event.Modifiers()&tcell.ModAlt != 0 {
		mod |= uv.ModAlt
	}
	if event.Modifiers()&tcell.ModCtrl != 0 {
		mod |= uv.ModCtrl
	}
	if event.Modifiers()&tcell.ModShift != 0 {
		mod |= uv.ModShift
	}
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
	if !s.closed && !s.Exited {
		s.Offset = 0
		s.Term.SendKey(terminalKey(event))
	}
}
