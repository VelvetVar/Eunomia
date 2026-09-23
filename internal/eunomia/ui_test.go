package eunomia

import (
	"context"
	"github.com/gdamore/tcell/v2"
	"strings"
	"testing"
	"time"
)

func uiFixture(t *testing.T) (*App, tcell.SimulationScreen) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	screen.SetSize(110, 38)
	a, err := NewApp(screen, Store{t.TempDir()}, false)
	if err != nil {
		t.Fatal(err)
	}
	a.Scan.Ranges = nil
	t.Cleanup(func() { a.Close(); screen.Fini() })
	return a, screen
}
func runeKey(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }
func press(a *App, key tcell.Key)    { a.HandleKey(tcell.NewEventKey(key, 0, tcell.ModNone)) }
func typeText(a *App, text string) {
	for _, r := range text {
		a.HandleKey(runeKey(r))
	}
}
func screenText(screen tcell.SimulationScreen) string {
	cells, w, h := screen.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := cells[y*w+x]
			if len(cell.Runes) > 0 {
				b.WriteRune(cell.Runes[0])
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
func TestTUIFormsSearchDetailsAndDelete(t *testing.T) {
	a, screen := uiFixture(t)
	a.Draw()
	if !strings.Contains(screenText(screen), "Your homelab starts here.") {
		t.Fatal(screenText(screen))
	}
	typeText(a, "aAtlas")
	press(a, tcell.KeyTAB)
	typeText(a, "127.0.0.1")
	press(a, tcell.KeyTAB)
	typeText(a, "admin")
	press(a, tcell.KeyEnter)
	if len(a.Devices) != 1 || a.Devices[0].Username != "admin" {
		t.Fatal(a.Message, a.Devices)
	}
	typeText(a, "e")
	press(a, tcell.KeyHome)
	typeText(a, "My ")
	press(a, tcell.KeyEnter)
	if a.Devices[0].Name != "My Atlas" {
		t.Fatal(a.Devices)
	}
	typeText(a, "/missing")
	if len(a.Filtered) != 0 {
		t.Fatal("search failed")
	}
	press(a, tcell.KeyEscape)
	typeText(a, "v")
	a.Draw()
	if !strings.Contains(screenText(screen), "DEVICE DETAILS") {
		t.Fatal("details")
	}
	press(a, tcell.KeyEscape)
	press(a, tcell.KeyDelete)
	press(a, tcell.KeyEscape)
	if len(a.Devices) != 1 {
		t.Fatal("cancel deleted")
	}
	press(a, tcell.KeyDelete)
	typeText(a, "y")
	if len(a.Devices) != 0 {
		t.Fatal("delete failed")
	}
}
func TestDiscoverUsesDAndPrefillsProfile(t *testing.T) {
	a, _ := uiFixture(t)
	typeText(a, "D")
	if a.View != -1 {
		t.Fatal("D did not select Discover")
	}
	a.Scan.Results = []Found{{Host: "192.168.1.44", Port: 22, Confirmed: true}}
	press(a, tcell.KeyEnter)
	if a.View != 0 || a.Mode != "form" || a.Form.Values[1] != "192.168.1.44" || a.Form.Field != 2 {
		t.Fatal(a.Form, a.Mode)
	}
	press(a, tcell.KeyEscape)
	press(a, tcell.KeyCtrlB)
	typeText(a, "d")
	if a.View != -1 {
		t.Fatal("prefix d")
	}
	press(a, tcell.KeyCtrlB)
	typeText(a, "0")
	if a.View != 0 {
		t.Fatal("prefix zero")
	}
	press(a, tcell.KeyF6)
	if a.View != -1 {
		t.Fatal("F6")
	}
}
func TestFingerprintConfirmationAndCancelledLookup(t *testing.T) {
	a, _ := uiFixture(t)
	d, err := a.Store.Save(fixtureDevice(), "")
	if err != nil {
		t.Fatal(err)
	}
	a.refresh()
	plan := HostKeyPlan{Device: d, Target: d.Host, Files: []string{"fixture-only"}}
	calls := 0
	a.prepare = func(context.Context, Device) (HostKeyPlan, error) { return plan, nil }
	a.forget = func(context.Context, HostKeyPlan, ToolRunner) error { calls++; return nil }
	typeText(a, "f")
	a.handleMessage(<-a.messages)
	if a.Pending == nil || a.Pending.Kind != "forget" {
		t.Fatal("no confirmation")
	}
	typeText(a, "n")
	if calls != 0 {
		t.Fatal("reset without confirmation")
	}
	typeText(a, "f")
	a.handleMessage(<-a.messages)
	typeText(a, "y")
	a.handleMessage(<-a.messages)
	if calls != 1 || len(a.Devices) != 1 || a.Mode != "list" {
		t.Fatal(calls, a.Mode)
	}
	typeText(a, "f")
	press(a, tcell.KeyEscape)
	a.handleMessage(<-a.messages)
	if a.Pending != nil {
		t.Fatal("cancelled lookup reopened dialog")
	}
}
func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestShutdownReclaimsUnclaimedSession(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "opening", true: "queued"}[queued], func(t *testing.T) {
			a, _ := uiFixture(t)
			p := newFakeTerminal()
			s := NewSession(fixtureDevice(), p, 80, 24, func(*Session) {})
			if queued {
				a.messages <- openedSession{Session: s}
			} else {
				a.Store.Save(fixtureDevice(), "")
				a.refresh()
				started := make(chan struct{})
				a.opener = func(Device, int, int, func(*Session)) (*Session, error) {
					close(started)
					<-a.ctx.Done()
					return s, nil
				}
				press(a, tcell.KeyEnter)
				<-started
			}
			a.Close()
			select {
			case <-p.done:
			default:
				t.Fatal("unclaimed terminal survived shutdown")
			}
		})
	}
}

func TestPasteCannotSaveDeleteOrRunMenuCommands(t *testing.T) {
	a, _ := uiFixture(t)
	d, err := a.Store.Save(fixtureDevice(), "")
	if err != nil {
		t.Fatal(err)
	}
	a.refresh()
	press(a, tcell.KeyDelete)
	a.handlePaste(true)
	typeText(a, "y")
	a.handlePaste(false)
	if a.Pending == nil || len(a.Devices) != 1 {
		t.Fatal("pasted text confirmed deletion")
	}
	press(a, tcell.KeyEscape)
	a.handlePaste(true)
	typeText(a, "qDa")
	a.handlePaste(false)
	if a.ctx.Err() != nil || a.Mode != "list" || a.View != 0 {
		t.Fatal("paste invoked menu shortcuts")
	}
	a.beginForm(d, true)
	a.Form.Field, a.Form.Cursor = 4, 0
	a.Form.Values[4] = ""
	a.handlePaste(true)
	typeText(a, "notes")
	press(a, tcell.KeyTAB)
	press(a, tcell.KeyEnter)
	typeText(a, "q")
	a.handlePaste(false)
	if a.Mode != "form" || a.Form.Field != 4 || a.Form.Values[4] != "notes  q" {
		t.Fatal("paste navigated or saved the form", a.Form)
	}
	stored, err := a.Store.Read()
	if err != nil || stored[0].Description != d.Description {
		t.Fatal("paste changed stored data", err)
	}
}

func TestSearchCursorAndLateSessionNotifications(t *testing.T) {
	a, _ := uiFixture(t)
	typeText(a, "/NXS")
	press(a, tcell.KeyLeft)
	press(a, tcell.KeyBackspace)
	typeText(a, "A")
	if a.Query != "NAS" {
		t.Fatal("search cursor reset between keys", a.Query)
	}
	p := newFakeTerminal()
	s := NewSession(fixtureDevice(), p, 80, 24, func(*Session) {})
	a.Sessions = []*Session{s}
	a.sessionChanged(s)
	a.closeSession(s)
	if a.sessionChanged(s) || len(a.sessionMarks) != 0 {
		t.Fatal("late notification retained a closed tab")
	}
}
