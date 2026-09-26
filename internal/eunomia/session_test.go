package eunomia

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"golang.org/x/term"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPTYHelper(t *testing.T) {
	if len(os.Args) < 3 || os.Args[len(os.Args)-2] != "--eunomia-helper" {
		return
	}
	switch os.Args[len(os.Args)-1] {
	case "echo":
		fmt.Printf("READY TTY=%v\r\n", term.IsTerminal(int(os.Stdin.Fd())))
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			fmt.Printf("GOT:%s\r\n", hex.EncodeToString(scanner.Bytes()))
		}
		os.Exit(0)
	case "ui":
		os.Exit(Main([]string{"up", "--no-animation"}, os.Stdout, os.Stderr))
	case "password":
		fmt.Print("PASSWORD READY:")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		if string(password) == "Dummy P@ss!#$%&*()_+-=42" {
			fmt.Print("\r\nPASSWORD ACCEPTED\r\n")
		} else {
			fmt.Print("\r\nPASSWORD REJECTED\r\n")
		}
		os.Exit(0)
	case "password-ui":
		screen, err := tcell.NewScreen()
		if err != nil {
			t.Fatal(err)
		}
		a, err := NewApp(screen, Store{ConfigDir()}, false)
		if err != nil {
			t.Fatal(err)
		}
		s := NewSession(fixtureDevice(), helperTerminal(t, "password", 80, 22), 80, 22, a.notify)
		a.Sessions, a.View = []*Session{s}, 1
		if err := a.Run(); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	case "menu":
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			t.Fatal(err)
		}
		defer term.Restore(int(os.Stdin.Fd()), state)
		for i := 0; i < 80; i++ {
			fmt.Printf("history %02d\r\n", i)
		}
		choices := []string{"First option", "Second option", "Third option"}
		selected := 0
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Printf("\r\x1b[2KChoose: %s", choices[selected])
			b, err := reader.ReadByte()
			if err != nil {
				return
			}
			if b == '\r' {
				fmt.Printf("\r\nConfirmed: %s\r\n", choices[selected])
				continue
			}
			if b != '\x1b' {
				continue
			}
			sequence := make([]byte, 2)
			if _, err := io.ReadFull(reader, sequence); err != nil {
				return
			}
			switch string(sequence) {
			case "[A":
				selected = max(0, selected-1)
			case "[B":
				selected = min(len(choices)-1, selected+1)
			}
		}
	case "scroll-ui":
		screen, err := tcell.NewScreen()
		if err != nil {
			t.Fatal(err)
		}
		a, err := NewApp(screen, Store{ConfigDir()}, false)
		if err != nil {
			t.Fatal(err)
		}
		s := NewSession(fixtureDevice(), newFakeTerminal(), 80, 22, func(*Session) {})
		s.mu.Lock()
		for i := 0; i < 80; i++ {
			fmt.Fprintf(s.Term, "line %02d\r\n", i)
		}
		s.mu.Unlock()
		a.Sessions, a.View = []*Session{s}, 1
		if err := a.Run(); err != nil {
			t.Fatal(err)
		}
		os.Exit(0)
	}
}
func helperTerminal(t *testing.T, mode string, w, h int) TerminalProcess {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := StartTerminal(exe, []string{"-test.run=^TestPTYHelper$", "--", "--eunomia-helper", mode}, w, h)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNativeTUIPasswordInput(t *testing.T) {
	for _, entry := range []struct{ name, input string }{
		{"typed", "Dummy P@ss!#$%&*()_+-=42\r"},
		{"paste", "\x1b[200~Dummy P@ss!#$%&*()_+-=42\x1b[201~\r"},
		{"backspace", "Dummy P@ss!#$%&*()_+-=4x\x7f2\r"},
	} {
		t.Run(entry.name, func(t *testing.T) {
			t.Setenv("EUNOMIA_HOME", t.TempDir())
			s := NewSession(fixtureDevice(), helperTerminal(t, "password-ui", 80, 24), 80, 24, func(*Session) {})
			defer s.Close()
			text := func() string { s.mu.Lock(); defer s.mu.Unlock(); return s.Term.String() }
			waitUntil(t, 8*time.Second, func() bool { return strings.Contains(text(), "PASSWORD READY:") })
			s.SendLiteral(entry.input)
			waitUntil(t, 3*time.Second, func() bool {
				return strings.Contains(text(), "PASSWORD ACCEPTED") || strings.Contains(text(), "PASSWORD REJECTED")
			})
			if !strings.Contains(text(), "PASSWORD ACCEPTED") {
				t.Fatal("dummy password was altered in transit")
			}
			if strings.Contains(text(), "Dummy P@ss") {
				t.Fatal("password was echoed")
			}
		})
	}
}
func TestNativeConcurrentTerminals(t *testing.T) {
	a := NewSession(fixtureDevice(), helperTerminal(t, "echo", 80, 24), 80, 24, func(*Session) {})
	b := NewSession(fixtureDevice(), helperTerminal(t, "echo", 80, 24), 80, 24, func(*Session) {})
	defer a.Close()
	defer b.Close()
	text := func(s *Session) string { s.mu.Lock(); defer s.mu.Unlock(); return s.Term.String() }
	waitUntil(t, 5*time.Second, func() bool {
		return strings.Contains(text(a), "READY TTY=true") && strings.Contains(text(b), "READY TTY=true")
	})
	a.SendLiteral("alpha\r")
	b.SendLiteral("beta\r")
	waitUntil(t, 5*time.Second, func() bool {
		return strings.Contains(text(a), "GOT:616c706861") && strings.Contains(text(b), "GOT:62657461")
	})
	if strings.Contains(text(a), "GOT:62657461") {
		t.Fatal("screen contamination")
	}
	a.Resize(100, 30)
	a.Close()
	b.SendLiteral("alive\r")
	waitUntil(t, 5*time.Second, func() bool { return strings.Contains(text(b), "GOT:616c697665") })
}
func TestRealTUIUpDownAndPersistence(t *testing.T) {
	t.Setenv("EUNOMIA_HOME", t.TempDir())
	p := helperTerminal(t, "ui", 110, 38)
	defer p.Close()
	var mu sync.Mutex
	var output strings.Builder
	go func() {
		buffer := make([]byte, 32768)
		for {
			n, err := p.Read(buffer)
			mu.Lock()
			output.Write(buffer[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	text := func() string { mu.Lock(); defer mu.Unlock(); return output.String() }
	deadline := time.Now().Add(8 * time.Second)
	for !strings.Contains(text(), "Your homelab starts here.") {
		if time.Now().After(deadline) {
			t.Fatalf("TUI failed: %s", text())
		}
		time.Sleep(20 * time.Millisecond)
	}
	p.Write([]byte("a"))
	waitUntil(t, 3*time.Second, func() bool { return strings.Contains(text(), "ADD A DEVICE") })
	p.Write([]byte("Test NAS\t127.0.0.1\tadmin\r"))
	store := Store{ConfigDir()}
	waitUntil(t, 3*time.Second, func() bool { devices, _ := store.Read(); return len(devices) == 1 })
	stopped, err := StopRunning(store.Directory)
	if !stopped || err != nil {
		t.Fatal(stopped, err)
	}
	done := make(chan int, 1)
	go func() { code, _ := p.Wait(); done <- code }()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatal("exit", code, text())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("down did not exit")
	}
	devices, err := store.Read()
	if err != nil || len(devices) != 1 || devices[0].Name != "Test NAS" {
		t.Fatal(devices, err)
	}
}

type fakeTerminal struct {
	reader *io.PipeReader
	writer *io.PipeWriter
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	input  strings.Builder
}

func newFakeTerminal() *fakeTerminal {
	r, w := io.Pipe()
	return &fakeTerminal{reader: r, writer: w, done: make(chan struct{})}
}
func (f *fakeTerminal) Read(b []byte) (int, error) { return f.reader.Read(b) }
func (f *fakeTerminal) Write(b []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.input.Write(b)
}
func (f *fakeTerminal) Resize(int, int) error { return nil }
func (f *fakeTerminal) Close() error {
	f.once.Do(func() { f.reader.Close(); f.writer.Close(); close(f.done) })
	return nil
}
func (f *fakeTerminal) Wait() (int, error) { <-f.done; return 0, nil }
func (f *fakeTerminal) inputText() string  { f.mu.Lock(); defer f.mu.Unlock(); return f.input.String() }
func TestRemoteKeysAltScreenScrollbackAndQuitPrompt(t *testing.T) {
	a, screen := uiFixture(t)
	p := newFakeTerminal()
	s := NewSession(fixtureDevice(), p, 110, 36, func(*Session) {})
	a.Sessions = []*Session{s}
	a.View = 1
	press(a, tcell.KeyTAB)
	press(a, tcell.KeyCtrlC)
	for _, key := range []tcell.Key{tcell.KeyCtrlBackslash, tcell.KeyCtrlRightSq, tcell.KeyCtrlCarat, tcell.KeyCtrlUnderscore} {
		press(a, key)
	}
	waitUntil(t, time.Second, func() bool { return p.inputText() == "\t\x03\x1c\x1d\x1e\x1f" })
	go p.writer.Write([]byte("SHELL\r\n\x1b[?1049h\x1b[2J\x1b[H\x1b[31mEDITOR\x1b[0m\x1b[?25l"))
	waitUntil(t, time.Second, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.Term.IsAltScreen() && strings.Contains(s.Term.String(), "EDITOR")
	})
	a.Draw()
	if !strings.Contains(screenText(screen), "EDITOR") {
		t.Fatal("alternate screen rendering")
	}
	s.mu.Lock()
	hidden := !s.CursorVisible
	s.mu.Unlock()
	if !hidden {
		t.Fatal("cursor visibility")
	}
	a.Pending = &action{Kind: "quit"}
	a.Draw()
	if !strings.Contains(screenText(screen), "Disconnect 1 active sessions") {
		t.Fatal("quit prompt")
	}
	typeText(a, "n")
	press(a, tcell.KeyCtrlB)
	typeText(a, "D")
	if a.View != -1 {
		t.Fatal("Discover prefix")
	}
	press(a, tcell.KeyCtrlB)
	typeText(a, "1")
	if a.View != 1 {
		t.Fatal("numbered tab")
	}
	go p.writer.Write([]byte("\x1b[?1049l\x1b[?25h\x1b[6n"))
	waitUntil(t, time.Second, func() bool { return strings.Contains(p.inputText(), "R") })
}

type blockedTerminal struct {
	*fakeTerminal
	started   chan struct{}
	startOnce sync.Once
}

func (p *blockedTerminal) Write([]byte) (int, error) {
	p.startOnce.Do(func() { close(p.started) })
	<-p.done
	return 0, io.ErrClosedPipe
}

func TestBlockedSSHInputDoesNotFreezeUI(t *testing.T) {
	p := &blockedTerminal{fakeTerminal: newFakeTerminal(), started: make(chan struct{})}
	s := NewSession(fixtureDevice(), p, 80, 24, func(*Session) {})
	defer s.Close()
	s.SendLiteral("first")
	<-p.started
	sent := make(chan struct{})
	go func() { s.SendLiteral("second"); s.SendLiteral("third"); close(sent) }()
	select {
	case <-sent:
	case <-time.After(time.Second):
		p.Close() // Allow cleanup even on the old synchronous implementation.
		<-sent
		t.Fatal("SSH backpressure blocked keyboard handling")
	}
}

func TestBlockedSSHInputHasBoundedBuffer(t *testing.T) {
	p := &blockedTerminal{fakeTerminal: newFakeTerminal(), started: make(chan struct{})}
	s := NewSession(fixtureDevice(), p, 80, 24, func(*Session) {})
	defer s.Close()
	sent := make(chan struct{})
	go func() { s.Paste(strings.Repeat("x", 2<<20)); close(sent) }()
	select {
	case <-sent:
	case <-time.After(3 * time.Second):
		p.Close()
		t.Fatal("blocked paste froze the emulator")
	}
	waitUntil(t, time.Second, func() bool { s.mu.Lock(); defer s.mu.Unlock(); return strings.Contains(s.Failure, "buffer full") })
	select {
	case <-p.done:
	case <-time.After(time.Second):
		t.Fatal("overflow did not close SSH")
	}
}

type shortWriteTerminal struct{ *fakeTerminal }

func (p shortWriteTerminal) Write(data []byte) (int, error) {
	return p.fakeTerminal.Write(data[:min(2, len(data))])
}

func TestSSHPastePreservesBytesAndHandlesShortWrites(t *testing.T) {
	a, _ := uiFixture(t)
	p := shortWriteTerminal{newFakeTerminal()}
	s := NewSession(fixtureDevice(), p, 80, 24, func(*Session) {})
	a.Sessions, a.View = []*Session{s}, 1
	s.mu.Lock()
	s.Term.Write([]byte("\x1b[?2004h"))
	s.mu.Unlock()
	a.handlePaste(true)
	typeText(a, "line one")
	press(a, tcell.KeyEnter)
	typeText(a, "line two")
	a.handlePaste(false)
	want := "\x1b[200~line one\nline two\x1b[201~"
	waitUntil(t, time.Second, func() bool { return p.inputText() == want })
}

func TestSSHArrowScrollback(t *testing.T) {
	for _, size := range [][2]int{{64, 24}, {110, 38}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			a, screen := uiFixture(t)
			screen.SetSize(size[0], size[1])
			p := newFakeTerminal()
			s := NewSession(fixtureDevice(), p, size[0], size[1]-2, func(*Session) {})
			a.Sessions, a.View = []*Session{s}, 1
			s.mu.Lock()
			for i := 0; i < 80; i++ {
				fmt.Fprintf(s.Term, "line %02d\r\n", i)
			}
			s.mu.Unlock()
			offset := func() int { s.mu.Lock(); defer s.mu.Unlock(); return s.Offset }
			a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
			a.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModAlt))
			waitUntil(t, time.Second, func() bool { return p.inputText() == "\x1b[A\x1b[B" })
			press(a, tcell.KeyUp)
			press(a, tcell.KeyUp)
			if offset() != 2 {
				t.Fatal("arrows did not scroll by line", offset())
			}
			a.Draw()
			if !strings.Contains(screenText(screen), "Scrollback -2") || !strings.Contains(screenText(screen), "line 78") || strings.Contains(screenText(screen), "line 79") {
				t.Fatal("scrollback viewport", screenText(screen))
			}
			press(a, tcell.KeyDown)
			if offset() != 1 {
				t.Fatal("down did not scroll toward live output")
			}
			press(a, tcell.KeyEscape)
			if offset() != 0 {
				t.Fatal("escape did not restore live output")
			}
			press(a, tcell.KeyPgUp)
			if offset() != size[1]-4 {
				t.Fatal("direct page up failed", offset())
			}
			for i := 0; i < 100; i++ {
				press(a, tcell.KeyUp)
			}
			s.mu.Lock()
			atTop := s.Offset == s.Term.ScrollbackLen()
			s.mu.Unlock()
			if !atTop {
				t.Fatal("scrolling did not stop at oldest line")
			}
			press(a, tcell.KeyEscape)
			press(a, tcell.KeyPgDn)
			if offset() != 0 {
				t.Fatal("page down moved below live output")
			}
			// Keep the old prefix shortcuts working as well.
			press(a, tcell.KeyCtrlB)
			press(a, tcell.KeyUp)
			press(a, tcell.KeyDown)
			if offset() != 0 {
				t.Fatal("down did not reach live output")
			}
			typeText(a, "z")
			waitUntil(t, time.Second, func() bool { return p.inputText() == "\x1b[A\x1b[Bz" })
			s.mu.Lock()
			s.Term.Write([]byte("\x1b[?1049h"))
			s.mu.Unlock()
			press(a, tcell.KeyCtrlB)
			press(a, tcell.KeyUp)
			press(a, tcell.KeyUp)
			waitUntil(t, time.Second, func() bool { return p.inputText() == "\x1b[A\x1b[Bz\x1b[A" })
			if offset() != 0 {
				t.Fatal("alternate screen scrolled local history")
			}
			s.mu.Lock()
			s.Term.Write([]byte("\x1b[?1049l"))
			s.Exited = true
			s.RemoteKeys = true
			s.mu.Unlock()
			press(a, tcell.KeyUp)
			if offset() != 1 {
				t.Fatal("exited tab did not scroll")
			}
		})
	}
}

func TestSSHMouseWheel(t *testing.T) {
	a, _ := uiFixture(t)
	p := newFakeTerminal()
	s := NewSession(fixtureDevice(), p, 110, 36, func(*Session) {})
	a.Sessions, a.View = []*Session{s}, 1
	s.mu.Lock()
	for i := 0; i < 80; i++ {
		fmt.Fprintf(s.Term, "line %02d\r\n", i)
	}
	s.mu.Unlock()
	offset := func() int { s.mu.Lock(); defer s.mu.Unlock(); return s.Offset }
	wheel := func(button tcell.ButtonMask) { a.HandleMouse(tcell.NewEventMouse(5, 4, button, tcell.ModNone)) }
	wheel(tcell.WheelUp)
	if offset() != 3 {
		t.Fatal("wheel did not enter scrollback", offset())
	}
	wheel(tcell.WheelUp)
	wheel(tcell.WheelDown)
	if offset() != 3 {
		t.Fatal("wheel direction", offset())
	}
	for i := 0; i < 100; i++ {
		wheel(tcell.WheelUp)
	}
	s.mu.Lock()
	atTop := s.Offset == s.Term.ScrollbackLen()
	s.mu.Unlock()
	if !atTop {
		t.Fatal("wheel did not clamp at top")
	}
	for i := 0; i < 100; i++ {
		wheel(tcell.WheelDown)
	}
	if offset() != 0 || p.inputText() != "" {
		t.Fatal("wheel leaked into shell or moved below bottom", offset(), p.inputText())
	}
	// Full-screen programs receive wheel events only when they request them.
	s.mu.Lock()
	s.Term.Write([]byte("\x1b[?1049h"))
	s.mu.Unlock()
	wheel(tcell.WheelUp)
	s.mu.Lock()
	s.Term.Write([]byte("\x1b[?1000h\x1b[?1006h"))
	s.mu.Unlock()
	wheel(tcell.WheelUp)
	wheel(tcell.WheelDown)
	press(a, tcell.KeyUp)
	press(a, tcell.KeyDown)
	press(a, tcell.KeyPgUp)
	press(a, tcell.KeyPgDn)
	a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	a.HandleKey(tcell.NewEventKey(tcell.KeyPgDn, 0, tcell.ModCtrl|tcell.ModShift))
	waitUntil(t, time.Second, func() bool {
		return p.inputText() == "\x1b[<64;6;4M\x1b[<65;6;4M\x1b[A\x1b[B\x1b[5~\x1b[6~\x1b[1;3A\x1b[6;6~"
	})
	if offset() != 0 {
		t.Fatal("wheel scrolled main history from alternate screen")
	}
}

func TestSelectionModeKeysAndTabIsolation(t *testing.T) {
	a, _ := uiFixture(t)
	p, other := newFakeTerminal(), newFakeTerminal()
	s := NewSession(fixtureDevice(), p, 110, 36, func(*Session) {})
	tab := NewSession(fixtureDevice(), other, 110, 36, func(*Session) {})
	a.Sessions, a.View = []*Session{s, tab}, 1
	press(a, tcell.KeyF7)
	press(a, tcell.KeyPgUp)
	press(a, tcell.KeyPgDn)
	a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	a.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModShift))
	press(a, tcell.KeyEscape)
	waitUntil(t, time.Second, func() bool { return p.inputText() == "\x1b[5~\x1b[6~\x1b[1;3A\x1b[1;2B\x1b" })
	press(a, tcell.KeyCtrlB)
	typeText(a, "2")
	press(a, tcell.KeyUp)
	typeText(a, "z")
	waitUntil(t, time.Second, func() bool { return other.inputText() == "z" })
	press(a, tcell.KeyCtrlB)
	typeText(a, "1")
	press(a, tcell.KeyDown)
	waitUntil(t, time.Second, func() bool { return strings.HasSuffix(p.inputText(), "\x1b[B") })
}

func TestNativeTUIScrolling(t *testing.T) {
	t.Setenv("EUNOMIA_HOME", t.TempDir())
	s := NewSession(fixtureDevice(), helperTerminal(t, "scroll-ui", 80, 24), 80, 24, func(*Session) {})
	defer s.Close()
	text := func() string { s.mu.Lock(); defer s.mu.Unlock(); return s.Term.String() }
	waitUntil(t, 8*time.Second, func() bool { return strings.Contains(text(), "line 79") })
	s.SendLiteral("\x1b[A")
	waitUntil(t, 3*time.Second, func() bool { return strings.Contains(text(), "Scrollback -1 |") })
	s.SendLiteral("\x1b[<64;6;5M")
	waitUntil(t, 3*time.Second, func() bool { return strings.Contains(text(), "Scrollback -4 |") })
	s.SendLiteral("\x1b[<65;6;5M")
	waitUntil(t, 3*time.Second, func() bool { return strings.Contains(text(), "Scrollback -1 |") })
	s.SendLiteral("\x1b[6~")
	waitUntil(t, 3*time.Second, func() bool { return !strings.Contains(text(), "Scrollback -") && strings.Contains(text(), "line 79") })
	if stopped, err := StopRunning(ConfigDir()); !stopped || err != nil {
		t.Fatal(stopped, err)
	}
}

func TestNativeArrowSelectionPrompt(t *testing.T) {
	a, screen := uiFixture(t)
	s := NewSession(fixtureDevice(), helperTerminal(t, "menu", 110, 36), 110, 36, func(*Session) {})
	a.Sessions, a.View = []*Session{s}, 1
	text := func() string { s.mu.Lock(); defer s.mu.Unlock(); return s.Term.String() }
	offset := func() int { s.mu.Lock(); defer s.mu.Unlock(); return s.Offset }
	waitUntil(t, 5*time.Second, func() bool { return strings.Contains(text(), "Choose: First option") })
	press(a, tcell.KeyUp)
	if offset() != 1 {
		t.Fatal("main-screen prompt prevented scrolling")
	}
	press(a, tcell.KeyF7)
	a.Draw()
	if offset() != 0 || !strings.Contains(screenText(screen), "SELECT: ↑/↓ remote options") {
		t.Fatal("selection mode was not visible", screenText(screen))
	}
	press(a, tcell.KeyDown)
	waitUntil(t, time.Second, func() bool { return strings.Contains(text(), "Choose: Second option") })
	a.HandleMouse(tcell.NewEventMouse(5, 10, tcell.WheelUp, tcell.ModNone))
	a.Draw()
	if offset() != 3 || !strings.Contains(screenText(screen), "SELECT: ↑/↓ remote") {
		t.Fatal("cannot scroll while selecting", screenText(screen))
	}
	press(a, tcell.KeyDown)
	waitUntil(t, time.Second, func() bool { return strings.Contains(text(), "Choose: Third option") })
	if offset() != 0 {
		t.Fatal("remote selection did not restore live prompt")
	}
	press(a, tcell.KeyUp)
	waitUntil(t, time.Second, func() bool { return strings.Contains(text(), "Choose: Second option") })
	press(a, tcell.KeyEnter)
	waitUntil(t, time.Second, func() bool { return strings.Contains(text(), "Confirmed: Second option") })
	press(a, tcell.KeyF7)
	press(a, tcell.KeyUp)
	if offset() != 1 {
		t.Fatal("could not return to arrow scrolling")
	}
	// A confirmation overlay must block mode changes and wheel input.
	a.Pending = &action{Kind: "close", Session: s}
	press(a, tcell.KeyF7)
	a.HandleMouse(tcell.NewEventMouse(5, 10, tcell.WheelUp, tcell.ModNone))
	s.mu.Lock()
	remote := s.RemoteKeys
	s.mu.Unlock()
	if remote || offset() != 1 || a.Pending == nil {
		t.Fatal("confirmation did not block navigation")
	}
}
