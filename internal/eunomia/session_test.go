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
