package eunomia

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func selectionFixture(t *testing.T, output string) (*App, tcell.SimulationScreen, *Session, *fakeTerminal) {
	t.Helper()
	a, screen := uiFixture(t)
	screen.SetSize(20, 6)
	p := newFakeTerminal()
	s := NewSession(fixtureDevice(), p, 20, 4, func(*Session) {})
	a.Sessions, a.View = []*Session{s}, 1
	s.mu.Lock()
	fmt.Fprint(s.Term, output)
	s.mu.Unlock()
	return a, screen, s, p
}

func selectionMouse(a *App, x, y int, buttons tcell.ButtonMask) {
	a.HandleMouse(tcell.NewEventMouse(x, y, buttons, tcell.ModNone))
}

func TestMouseSelectionRightClickCopiesWithoutPasting(t *testing.T) {
	a, screen, s, p := selectionFixture(t, "first line\r\n  second line")
	var copied []string
	a.writeClipboard = func(text string) error { copied = append(copied, text); return nil }
	reads := 0
	a.clipboard = func() (string, error) { reads++; return copied[len(copied)-1], nil }
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 12, 2, tcell.ButtonPrimary)
	a.Draw()
	_, _, selectedStyle, _ := screen.GetContent(1, 1)
	_, _, outsideStyle, _ := screen.GetContent(14, 2)
	_, _, selectedAttrs := selectedStyle.Decompose()
	_, _, outsideAttrs := outsideStyle.Decompose()
	if selectedAttrs&tcell.AttrReverse == 0 || outsideAttrs&tcell.AttrReverse != 0 || len(copied) != 0 {
		t.Fatal("selection highlight or release-to-copy behavior is incorrect")
	}
	s.mu.Lock()
	fmt.Fprint(s.Term, "\x1b[2J\x1b[Hnew output")
	s.mu.Unlock()
	a.Draw()
	if !strings.Contains(screenText(screen), "first line") || strings.Contains(screenText(screen), "new output") {
		t.Fatal("incoming output changed the selection snapshot")
	}
	selectionMouse(a, 12, 2, tcell.ButtonNone)
	selectionMouse(a, 12, 2, tcell.ButtonNone)
	if len(copied) != 1 || copied[0] != "first line\n  second line" || p.inputText() != "" {
		t.Fatal("copy changed text, repeated, or sent input to SSH", copied)
	}
	selectionMouse(a, 5, 2, tcell.ButtonSecondary)
	selectionMouse(a, 6, 2, tcell.ButtonSecondary)
	selectionMouse(a, 5, 2, tcell.ButtonNone)
	if a.selection == nil || len(copied) != 2 || copied[1] != copied[0] || reads != 0 || p.inputText() != "" {
		t.Fatal("right-click changed the selection or attempted to paste", copied, reads)
	}
	selectionMouse(a, 5, 2, tcell.ButtonSecondary)
	selectionMouse(a, 5, 2, tcell.ButtonNone)
	if a.selection == nil || len(copied) != 3 || copied[2] != copied[0] || reads != 0 || p.inputText() != "" {
		t.Fatal("repeated right-click pasted selected text", copied, reads)
	}
	press(a, tcell.KeyEscape)
	selectionMouse(a, 5, 2, tcell.ButtonSecondary)
	selectionMouse(a, 5, 2, tcell.ButtonNone)
	waitUntil(t, time.Second, func() bool { return p.inputText() != "" })
	if a.selection != nil || reads != 1 || !strings.Contains(p.inputText(), "first line") || !strings.Contains(p.inputText(), "  second line") {
		t.Fatal("right-click did not paste after explicitly clearing the selection")
	}
	a.Draw()
	if !strings.Contains(screenText(screen), "new output") {
		t.Fatal("clearing selection did not restore current output")
	}
}

func TestRightClickDuringSelectionOrCopyFailureNeverPastes(t *testing.T) {
	for _, failCopy := range []bool{false, true} {
		t.Run(fmt.Sprintf("copyFailure=%v", failCopy), func(t *testing.T) {
			a, _, _, p := selectionFixture(t, "copy me")
			copies := 0
			a.writeClipboard = func(text string) error {
				copies++
				if text != "copy me" {
					t.Fatal("right-click moved the selection endpoint", text)
				}
				if failCopy {
					return errors.New("clipboard busy")
				}
				return nil
			}
			a.clipboard = func() (string, error) { t.Fatal("read clipboard while text was selected"); return "", nil }
			selectionMouse(a, 0, 1, tcell.ButtonPrimary)
			selectionMouse(a, 6, 1, tcell.ButtonPrimary)
			selectionMouse(a, 12, 2, tcell.ButtonSecondary)
			selectionMouse(a, 12, 2, tcell.ButtonNone)
			selectionMouse(a, 12, 2, tcell.ButtonSecondary)
			selectionMouse(a, 12, 2, tcell.ButtonNone)
			if a.selection == nil || a.selection.dragging || copies != 2 || p.inputText() != "" {
				t.Fatal("right-click lost the selection or pasted during copy", copies)
			}
			a.pasteClipboard()
		})
	}
}

func TestMouseSelectionUnicodeReverseAndConcealedText(t *testing.T) {
	a, _, _, _ := selectionFixture(t, "A界e\u0301Z\r\nshow\x1b[8mSECRET\x1b[28mend")
	var copied string
	a.writeClipboard = func(text string) error { copied = text; return nil }
	// Start on a wide character's trailing cell and drag backwards.
	selectionMouse(a, 3, 1, tcell.ButtonPrimary)
	selectionMouse(a, 2, 1, tcell.ButtonPrimary)
	selectionMouse(a, 2, 1, tcell.ButtonNone)
	if copied != "界e\u0301" {
		t.Fatal("wide or combining character was split", copied)
	}
	selectionMouse(a, 12, 2, tcell.ButtonPrimary)
	selectionMouse(a, 0, 2, tcell.ButtonNone)
	if copied != "show      end" {
		t.Fatal("concealed terminal text was exposed or reverse selection failed", copied)
	}
}

func TestMouseSelectionScrollbackAlternateAndExitedTabs(t *testing.T) {
	a, _, s, _ := selectionFixture(t, "")
	var copied string
	a.writeClipboard = func(text string) error { copied = text; return nil }
	s.mu.Lock()
	for i := 0; i < 12; i++ {
		fmt.Fprintf(s.Term, "line %02d\r\n", i)
	}
	s.Offset = s.Term.ScrollbackLen()
	s.mu.Unlock()
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 6, 1, tcell.ButtonNone)
	if copied != "line 00" {
		t.Fatal("selection read live rows instead of visible scrollback", copied)
	}
	s.mu.Lock()
	fmt.Fprint(s.Term, "\x1b[?1049h\x1b[HALTERNATE")
	s.mu.Unlock()
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 8, 1, tcell.ButtonNone)
	if copied != "ALTERNATE" {
		t.Fatal("selection did not use alternate-screen content", copied)
	}
	s.Close()
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 3, 1, tcell.ButtonNone)
	if copied != "ALTE" {
		t.Fatal("cannot copy from an exited tab", copied)
	}
}

func TestMouseSelectionCancellationAndGuards(t *testing.T) {
	a, screen, _, p := selectionFixture(t, "copy me")
	copies := 0
	a.writeClipboard = func(string) error { copies++; return nil }
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 0, 1, tcell.ButtonNone)
	if copies != 0 || a.selection != nil {
		t.Fatal("a plain click overwrote the clipboard")
	}
	for _, guard := range []string{"busy", "pending", "prefix", "paste"} {
		a.Busy, a.Prefix, a.paste = guard == "busy", guard == "prefix", guard == "paste"
		if guard == "pending" {
			a.Pending = &action{Kind: "close"}
		}
		selectionMouse(a, 0, 1, tcell.ButtonPrimary)
		selectionMouse(a, 6, 1, tcell.ButtonNone)
		a.Pending = nil
	}
	a.Busy, a.Prefix, a.paste = false, false, false
	if copies != 0 || a.selection != nil {
		t.Fatal("selection bypassed a modal input guard")
	}
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 6, 1, tcell.ButtonPrimary)
	press(a, tcell.KeyEscape)
	selectionMouse(a, 6, 1, tcell.ButtonNone)
	if copies != 0 || a.selection != nil || p.inputText() != "" {
		t.Fatal("Escape did not cancel selection locally")
	}
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 6, 1, tcell.ButtonNone)
	press(a, tcell.KeyCtrlC)
	waitUntil(t, time.Second, func() bool { return p.inputText() == "\x03" })
	if a.selection != nil {
		t.Fatal("Ctrl+C did not clear selection")
	}
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 6, 1, tcell.ButtonPrimary)
	a.activate(0)
	selectionMouse(a, 6, 1, tcell.ButtonNone)
	if a.selection != nil || copies != 1 {
		t.Fatal("switching tabs copied an unfinished selection")
	}
	a.activate(1)
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	screen.SetSize(21, 6)
	a.Draw()
	selectionMouse(a, 6, 1, tcell.ButtonNone)
	if a.selection != nil || copies != 1 {
		t.Fatal("resize retained stale selection coordinates")
	}
}

func TestMouseSelectionClipboardFailureFallbackAndPrivacy(t *testing.T) {
	a, screen, _, p := selectionFixture(t, "DUMMY-copy-output")
	a.writeClipboard = func(string) error { return errors.New("clipboard busy") }
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 16, 1, tcell.ButtonNone)
	if a.selection == nil || a.selection.status != "Copy failed: clipboard busy" || p.inputText() != "" {
		t.Fatal("clipboard error was hidden or sent to SSH")
	}
	a.writeClipboard = nil
	selectionMouse(a, 0, 1, tcell.ButtonPrimary)
	selectionMouse(a, 16, 1, tcell.ButtonNone)
	if string(screen.GetClipboardData()) != "DUMMY-copy-output" {
		t.Fatal("terminal clipboard fallback changed copied text")
	}
	files, _ := os.ReadDir(a.Diagnostics.Directory)
	for _, file := range files {
		data, err := os.ReadFile(filepath.Join(a.Diagnostics.Directory, file.Name()))
		if err != nil || strings.Contains(string(data), "DUMMY-") {
			t.Fatal("copied output leaked to diagnostics", err)
		}
	}
}

func TestNativeTUIMouseSelection(t *testing.T) {
	t.Setenv("EUNOMIA_HOME", t.TempDir())
	s := NewSession(fixtureDevice(), helperTerminal(t, "copy-ui", 80, 24), 80, 24, func(*Session) {})
	defer s.Close()
	text := func() string { s.mu.Lock(); defer s.mu.Unlock(); return s.Term.String() }
	waitUntil(t, 8*time.Second, func() bool { return strings.Contains(text(), "COPY ME") })
	s.SendLiteral("\x1b[<0;1;2M\x1b[<32;7;2M")
	waitUntil(t, 3*time.Second, func() bool { return strings.Contains(text(), "Selecting text") })
	s.SendLiteral("\x1b[<0;7;2m")
	waitUntil(t, 3*time.Second, func() bool { return strings.Contains(text(), "Copied selection") })
	s.SendLiteral("\x1b")
	waitUntil(t, 3*time.Second, func() bool { return !strings.Contains(text(), "Copied selection") })
	if stopped, err := StopRunning(ConfigDir()); !stopped || err != nil {
		t.Fatal(stopped, err)
	}
}

func TestMouseSelectionAtTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{64, 24}, {110, 38}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			a, screen := uiFixture(t)
			screen.SetSize(size[0], size[1])
			s := NewSession(fixtureDevice(), newFakeTerminal(), size[0], size[1]-2, func(*Session) {})
			a.Sessions, a.View = []*Session{s}, 1
			s.mu.Lock()
			fmt.Fprint(s.Term, "select this text")
			s.mu.Unlock()
			var copied string
			a.writeClipboard = func(text string) error { copied = text; return nil }
			selectionMouse(a, 0, 1, tcell.ButtonPrimary)
			selectionMouse(a, 15, 1, tcell.ButtonPrimary)
			a.Draw()
			if !strings.Contains(screenText(screen), "release to copy") {
				t.Fatal("selection instructions were not visible")
			}
			// A release past the right edge clamps to the SSH content.
			selectionMouse(a, size[0]+10, 1, tcell.ButtonNone)
			a.Draw()
			if copied != "select this text" || !strings.Contains(screenText(screen), "Copied selection") {
				t.Fatal("selection copied chrome or failed at terminal boundary")
			}
		})
	}
}
