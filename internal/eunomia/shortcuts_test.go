package eunomia

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestCommandBarStaysVisibleAndMatchesContext(t *testing.T) {
	for _, size := range [][2]int{{64, 24}, {80, 24}, {110, 38}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			a, screen := uiFixture(t)
			screen.SetSize(size[0], size[1])
			device := saveLabDevice(t, a.Store, "Atlas")
			folder, err := a.Store.SaveFolder("Servers", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := a.refresh(); err != nil {
				t.Fatal(err)
			}
			assertFooter := func(want ...string) string {
				t.Helper()
				a.Draw()
				lines := strings.Split(screenText(screen), "\n")
				footer := strings.Join(lines[a.commandBarTop(size[0], size[1]):], "\n")
				for _, label := range want {
					if !strings.Contains(footer, label) {
						t.Fatalf("missing footer shortcut %q:\n%s", label, footer)
					}
				}
				return footer
			}
			a.selectDevice(device.ID)
			a.Message = "Device saved."
			assertFooter("Device saved.", "Shift+M Move menu", "Shift+F New folder", "m Animation", "f Forget fingerprint", "? All keys", "Ctrl+B Tab commands", "Enter Connect")
			a.Error, a.Message = true, "Cannot move any further."
			assertFooter("Cannot move any further.", "Shift+M Move menu", "Alt+↑/↓ Move up/down")
			a.selectFolder(folder.ID)
			footer := assertFooter("Shift+M Move folder", "e Rename folder", "Enter/Space Expand/collapse")
			if strings.Contains(footer, "Enter Connect") || strings.Contains(footer, "f Forget") {
				t.Fatal("device actions advertised for a folder")
			}
			a.Mode, a.Message, a.Error = "form", "", false
			assertFooter("Enter Save device", "Shift+Tab/↑ Previous field", "Ctrl+U Clear field", "Esc Cancel")
			a.Mode = "folder"
			assertFooter("Enter Save folder", "Esc Cancel")
			a.Mode = "search"
			assertFooter("Enter Apply search", "Esc Clear search")
			a.Mode = "move"
			assertFooter("↑/↓ Choose", "Enter Move", "Esc Cancel")
			a.View = -1
			assertFooter("Enter/a Add/select device", "c Custom subnet", "r Scan", "x Stop scan", "? All keys")
			a.Scan.Editing = true
			assertFooter("Enter Use subnet and scan", "Esc Cancel")
			a.Pending = &action{Kind: "delete", Device: device}
			footer = assertFooter("y Confirm", "n/Esc Cancel")
			if strings.Contains(footer, "Save device") {
				t.Fatal("confirmation advertises inactive commands")
			}
		})
	}
}

func TestShortcutCaseAndHelpPreserveEditing(t *testing.T) {
	a, screen := uiFixture(t)
	device := saveLabDevice(t, a.Store, "Atlas")
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	a.selectDevice(device.ID)
	before := a.Animate
	typeText(a, "m")
	if a.Mode != "list" || a.Animate == before {
		t.Fatal("plain m must toggle animation")
	}
	a.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'M', tcell.ModShift))
	if a.Mode != "move" {
		t.Fatal("Shift+M must open Move")
	}
	press(a, tcell.KeyEscape)
	a.HandleKey(tcell.NewEventKey(tcell.KeyRune, 'F', tcell.ModShift))
	typeText(a, "Unsaved folder")
	press(a, tcell.KeyCtrlB)
	typeText(a, "?")
	if !a.HelpOpen {
		t.Fatal("Ctrl+B then ? did not open help")
	}
	screen.SetSize(64, 24)
	var pages strings.Builder
	for page := 0; page < a.helpPageCount(64, 24); page++ {
		a.Draw()
		view := screenText(screen)
		if !strings.Contains(view, "Esc Back") || !strings.Contains(view, "Shift+M = hold Shift") {
			t.Fatal("help navigation or case guidance missing")
		}
		pages.WriteString(view)
		press(a, tcell.KeyPgDn)
	}
	for _, label := range []string{"Shift+M  Move menu", "Shift+F  New folder", "Right-click with selection", "Send Ctrl+B", "Custom subnet"} {
		if !strings.Contains(pages.String(), label) {
			t.Fatal("help omitted a shortcut", label)
		}
	}
	// A background fingerprint check can request confirmation while help is open.
	// Guide keystrokes must never confirm a prompt hidden underneath it.
	a.Pending = &action{Kind: "delete", Device: device}
	typeText(a, "y")
	if a.Pending == nil {
		t.Fatal("guide input confirmed a hidden action")
	}
	a.handlePaste(true)
	typeText(a, "overwritten\n")
	a.handlePaste(false)
	press(a, tcell.KeyEscape)
	if a.HelpOpen || a.Mode != "folder" || a.FolderForm.Name != "Unsaved folder" {
		t.Fatal("guide changed underlying form")
	}
	typeText(a, "n")
}

func TestSSHGuideDoesNotSendRemoteInput(t *testing.T) {
	a, screen, _, terminal := selectionFixture(t, "Ready")
	screen.SetSize(64, 24)
	press(a, tcell.KeyCtrlB)
	a.Draw()
	if !strings.Contains(screenText(screen), "release Ctrl+B, then press a key") || !strings.Contains(screenText(screen), "x Close SSH tab") {
		t.Fatal("SSH prefix instructions clipped or ambiguous")
	}
	typeText(a, "?")
	press(a, tcell.KeyPgDn)
	a.handlePaste(true)
	typeText(a, "do not send\n")
	a.handlePaste(false)
	press(a, tcell.KeyEscape)
	terminal.mu.Lock()
	defer terminal.mu.Unlock()
	if terminal.input.Len() != 0 {
		t.Fatal("shortcut guide sent input to SSH")
	}
	if a.View != 1 || a.HelpOpen {
		t.Fatal("closing help did not return to SSH")
	}
}
