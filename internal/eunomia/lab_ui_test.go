package eunomia

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func TestLabMoveFoldersCollapseSearchAndDelete(t *testing.T) {
	a, screen := uiFixture(t)
	alpha := saveLabDevice(t, a.Store, "Alpha")
	beta := saveLabDevice(t, a.Store, "Beta")
	gamma := saveLabDevice(t, a.Store, "Gamma")
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	typeText(a, "M")
	press(a, tcell.KeyDown)
	press(a, tcell.KeyEnter)
	rows := a.labRows()
	if rows[0].Device.ID != beta.ID || rows[1].Device.ID != alpha.ID || a.Selected != 1 {
		t.Fatal("Move down did not preserve device selection")
	}
	a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	if a.labRows()[0].Device.ID != alpha.ID || a.Selected != 0 {
		t.Fatal("Alt+Up did not reorder")
	}
	typeText(a, "FServers")
	press(a, tcell.KeyEnter)
	folder, ok := a.selectedFolder()
	if !ok || folder.Name != "Servers" {
		t.Fatal("folder not created or selected", a.Message)
	}
	a.selectDevice(gamma.ID)
	typeText(a, "M")
	for i := 0; i < 3; i++ {
		press(a, tcell.KeyDown)
	}
	press(a, tcell.KeyEnter)
	selected, ok := a.selected()
	if !ok || selected.ID != gamma.ID || a.Layout.DeviceFolders[gamma.ID] != folder.ID {
		t.Fatal("Move to folder failed", a.Message)
	}
	// Left on a child collapses the parent; its heading remains selectable.
	press(a, tcell.KeyLeft)
	if f, ok := a.selectedFolder(); !ok || !f.Collapsed || len(a.labRows()) != 3 {
		t.Fatal("folder did not collapse")
	}
	a.Draw()
	if !strings.Contains(screenText(screen), "[+] Servers (1)") {
		t.Fatal("collapsed folder heading or count is missing")
	}
	reloaded, err := a.Store.ReadLayout()
	if err != nil || !reloaded.Folders[0].Collapsed {
		t.Fatal("collapsed state did not persist", err)
	}
	typeText(a, "/Gamma")
	if selected, ok := a.selected(); !ok || selected.ID != gamma.ID || len(a.labRows()) != 1 {
		t.Fatal("search could not find a collapsed device")
	}
	press(a, tcell.KeyEscape)
	a.selectFolder(folder.ID)
	press(a, tcell.KeyRight)
	if len(a.labRows()) != 4 || a.Opening {
		t.Fatal("expanding folder opened SSH or lost children")
	}
	typeText(a, "e")
	press(a, tcell.KeyCtrlU)
	typeText(a, "Homelab")
	press(a, tcell.KeyEnter)
	if f, ok := a.selectedFolder(); !ok || f.ID != folder.ID || f.Name != "Homelab" {
		t.Fatal("folder rename failed")
	}
	press(a, tcell.KeyDelete)
	if a.Pending == nil || a.Pending.Kind != "delete-folder" || !strings.Contains(a.prompt(), "Keep devices") {
		t.Fatal("folder deletion did not explain that devices are kept")
	}
	typeText(a, "n")
	if len(a.Layout.Folders) != 1 {
		t.Fatal("cancel deleted the folder")
	}
	press(a, tcell.KeyDelete)
	typeText(a, "y")
	if len(a.Layout.Folders) != 0 || len(a.Devices) != 3 || len(a.labRows()) != 3 {
		t.Fatal("deleting folder deleted or hid device profiles", a.Message)
	}
}

func TestLabAddInsideFolderAndMoveBackToRoot(t *testing.T) {
	a, _ := uiFixture(t)
	typeText(a, "FServers")
	press(a, tcell.KeyEnter)
	folder, _ := a.selectedFolder()
	typeText(a, "aNew server")
	press(a, tcell.KeyTAB)
	typeText(a, "127.0.0.1")
	press(a, tcell.KeyTAB)
	typeText(a, "admin")
	press(a, tcell.KeyEnter)
	device, ok := a.selected()
	if !ok || a.Layout.DeviceFolders[device.ID] != folder.ID || len(a.Devices) != 1 {
		t.Fatal("adding inside selected folder failed", a.Message)
	}
	typeText(a, "e")
	press(a, tcell.KeyCtrlU)
	typeText(a, "Renamed server")
	press(a, tcell.KeyEnter)
	if a.Layout.DeviceFolders[device.ID] != folder.ID || a.Devices[0].Name != "Renamed server" {
		t.Fatal("editing a device changed folder membership")
	}
	typeText(a, "M")
	press(a, tcell.KeyDown)
	press(a, tcell.KeyDown)
	press(a, tcell.KeyEnter)
	if a.Layout.DeviceFolders[device.ID] != "" || a.labRows()[0].Device.ID != device.ID {
		t.Fatal("Move back to Lab failed", a.Message)
	}
	if len(a.Layout.Folders) != 1 || a.labRows()[1].Count != 0 {
		t.Fatal("moving last device removed its folder")
	}
}

func TestLabFolderPasteAndMoveCancellation(t *testing.T) {
	a, _ := uiFixture(t)
	device := saveLabDevice(t, a.Store, "Device")
	a.refresh()
	typeText(a, "F")
	a.clipboard = func() (string, error) { return "Servers\tq\r\n", nil }
	rightClick(a)
	if a.Mode != "folder" || a.FolderForm.Name != "Servers q  " || a.ctx.Err() != nil {
		t.Fatal("clipboard paste submitted the folder form or ran shortcuts")
	}
	if _, err := os.Stat(a.Store.LayoutPath()); !os.IsNotExist(err) {
		t.Fatal("pasted newline saved a folder")
	}
	press(a, tcell.KeyEnter)
	folder, _ := a.selectedFolder()
	press(a, tcell.KeyDelete)
	a.handlePaste(true)
	typeText(a, "y")
	a.handlePaste(false)
	if a.Pending == nil || len(a.Layout.Folders) != 1 {
		t.Fatal("paste confirmed a folder deletion")
	}
	press(a, tcell.KeyEscape)
	a.selectDevice(device.ID)
	original, _ := os.ReadFile(a.Store.LayoutPath())
	typeText(a, "M")
	press(a, tcell.KeyDown)
	press(a, tcell.KeyDown)
	press(a, tcell.KeyDown)
	press(a, tcell.KeyEscape)
	after, _ := os.ReadFile(a.Store.LayoutPath())
	if string(original) != string(after) || a.Mode != "list" || a.Layout.DeviceFolders[device.ID] == folder.ID {
		t.Fatal("cancelling Move changed organization")
	}
}

func TestLabFolderLayoutAtTerminalSizes(t *testing.T) {
	for _, size := range [][2]int{{64, 24}, {110, 38}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			a, screen := uiFixture(t)
			screen.SetSize(size[0], size[1])
			device := saveLabDevice(t, a.Store, "Atlas")
			folder, err := a.Store.SaveFolder("Servers", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := a.Store.PlaceDevice(device.ID, folder.ID); err != nil {
				t.Fatal(err)
			}
			a.refresh()
			a.selectFolder(folder.ID)
			a.Draw()
			view := screenText(screen)
			if !strings.Contains(view, "[-] Servers (1)") || !strings.Contains(view, "M move") || !strings.Contains(view, "F folder") {
				t.Fatal("folder heading or organization controls clipped", view)
			}
			a.selectDevice(device.ID)
			typeText(a, "M")
			a.Draw()
			view = screenText(screen)
			for _, label := range []string{"MOVE DEVICE", "Move up", "Move down", "Move to Lab", "Move to folder: Servers"} {
				if !strings.Contains(view, label) {
					t.Fatal("Move option not visible", label, view)
				}
			}
			press(a, tcell.KeyEscape)
			typeText(a, "F")
			a.Draw()
			if !strings.Contains(screenText(screen), "CREATE FOLDER") || !strings.Contains(screenText(screen), "Enter save folder") {
				t.Fatal("folder form is not usable at terminal size")
			}
		})
	}
}

func TestNativeLabOrganization(t *testing.T) {
	t.Setenv("EUNOMIA_HOME", t.TempDir())
	store := Store{ConfigDir()}
	device := fixtureDevice()
	device.Name, device.Host = "Alpha", "127.0.0.1"
	if _, err := store.Save(device, ""); err != nil {
		t.Fatal(err)
	}
	device.Name = "Beta"
	beta, err := store.Save(device, "")
	if err != nil {
		t.Fatal(err)
	}
	session := NewSession(fixtureDevice(), helperTerminal(t, "ui", 80, 24), 80, 24, func(*Session) {})
	defer session.Close()
	text := func() string { session.mu.Lock(); defer session.mu.Unlock(); return session.Term.String() }
	waitUntil(t, 8*time.Second, func() bool { return strings.Contains(text(), "Alpha") && strings.Contains(text(), "Beta") })
	session.SendLiteral("FServers\r")
	var folderID string
	waitUntil(t, 3*time.Second, func() bool {
		layout, err := store.ReadLayout()
		if err != nil || len(layout.Folders) != 1 {
			return false
		}
		folderID = layout.Folders[0].ID
		return strings.Contains(text(), "Servers (0)")
	})
	session.SendLiteral("/Beta\rM")
	waitUntil(t, 3*time.Second, func() bool { return strings.Contains(text(), "MOVE DEVICE") })
	session.SendLiteral("\x1b[B\x1b[B\x1b[B\r")
	waitUntil(t, 3*time.Second, func() bool {
		layout, err := store.ReadLayout()
		return err == nil && layout.DeviceFolders[beta.ID] == folderID && strings.Contains(text(), "Servers (1)")
	})
	session.SendLiteral("\x1b[D")
	waitUntil(t, 3*time.Second, func() bool {
		layout, err := store.ReadLayout()
		return err == nil && layout.Folders[0].Collapsed && strings.Contains(text(), "[+] Servers")
	})
	session.SendLiteral("\r")
	waitUntil(t, 3*time.Second, func() bool {
		layout, err := store.ReadLayout()
		return err == nil && !layout.Folders[0].Collapsed && strings.Contains(text(), "[-] Servers")
	})
	session.SendLiteral("q")
	waitUntil(t, 3*time.Second, func() bool { session.mu.Lock(); defer session.mu.Unlock(); return session.Exited })
	session.mu.Lock()
	code := session.ExitCode
	session.mu.Unlock()
	if code != 0 {
		t.Fatal("native Lab exited with an error", code)
	}
}
