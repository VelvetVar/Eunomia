package eunomia

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestFoldersMoveWithMembersAnywhereInLab(t *testing.T) {
	a, screen := uiFixture(t)
	alpha := saveLabDevice(t, a.Store, "Alpha")
	beta := saveLabDevice(t, a.Store, "Beta")
	child := saveLabDevice(t, a.Store, "Child")
	one, err := a.Store.SaveFolder("One", "")
	if err != nil {
		t.Fatal(err)
	}
	two, err := a.Store.SaveFolder("Two", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PlaceDevice(child.ID, one.ID); err != nil {
		t.Fatal(err)
	}
	profiles, _ := os.ReadFile(a.Store.Path())
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	assertRows := func(want string) {
		t.Helper()
		if a.Error {
			t.Fatal(a.Message)
		}
		var names []string
		for _, row := range a.labRows() {
			if row.IsFolder {
				names = append(names, "["+row.Folder.Name+"]")
			} else {
				names = append(names, row.Device.Name)
			}
		}
		if strings.Join(names, ",") != want {
			t.Fatalf("got %v, want %s", names, want)
		}
		if a.Layout.DeviceFolders[child.ID] != one.ID {
			t.Fatal("moving folder reassigned a member")
		}
	}
	a.selectFolder(one.ID)
	a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	assertRows("Alpha,[One],Child,Beta,[Two]")
	if folder, ok := a.selectedFolder(); !ok || folder.ID != one.ID {
		t.Fatal("folder selection lost")
	}
	typeText(a, "M")
	a.Draw()
	if !strings.Contains(screenText(screen), "MOVE FOLDER") || !strings.Contains(screenText(screen), "all its devices move together") {
		t.Fatal("folder move menu unclear")
	}
	press(a, tcell.KeyEnter)
	assertRows("[One],Child,Alpha,Beta,[Two]")
	beforeEdge, _ := os.ReadFile(a.Store.LayoutPath())
	if err := a.Store.MoveFolder(one.ID, -1); err == nil {
		t.Fatal("folder moved past the top of Lab")
	}
	afterEdge, _ := os.ReadFile(a.Store.LayoutPath())
	if !bytes.Equal(beforeEdge, afterEdge) {
		t.Fatal("failed folder move rewrote the layout")
	}
	press(a, tcell.KeyEnter) // Collapse, then move the whole folder down.
	a.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModAlt))
	assertRows("Alpha,[One],Beta,[Two]")
	if !a.Layout.Folders[0].Collapsed {
		t.Fatal("moving folder changed collapsed state")
	}
	a.selectFolder(two.ID)
	for i := 0; i < 2; i++ {
		a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	}
	assertRows("Alpha,[Two],[One],Beta")
	reopened, _ := uiFixture(t)
	reopened.Store = a.Store
	if err := reopened.refresh(); err != nil {
		t.Fatal(err)
	}
	if len(reopened.labRows()) != 4 || reopened.labRows()[1].Folder.ID != two.ID || reopened.labRows()[3].Device.ID != beta.ID || reopened.labRows()[0].Device.ID != alpha.ID {
		t.Fatal("mixed folder order did not persist")
	}
	if err := a.Store.DeleteFolder(one.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	rows := a.labRows()
	if rows[2].Device.ID != child.ID || rows[3].Device.ID != beta.ID {
		t.Fatal("deleting a moved folder lost its children's position")
	}
	after, _ := os.ReadFile(a.Store.Path())
	if !bytes.Equal(profiles, after) {
		t.Fatal("moving/deleting folders modified profiles")
	}
}

func TestDeviceMovesAcrossMixedRootItems(t *testing.T) {
	a, _ := uiFixture(t)
	alpha := saveLabDevice(t, a.Store, "Alpha")
	beta := saveLabDevice(t, a.Store, "Beta")
	child := saveLabDevice(t, a.Store, "Child")
	folder, err := a.Store.SaveFolder("Servers", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Store.PlaceDevice(child.ID, folder.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.Store.MoveFolder(folder.ID, -1); err != nil {
		t.Fatal(err)
	}
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	a.selectDevice(child.ID)
	a.HandleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModAlt))
	rows := a.labRows()
	if a.Error || a.Layout.DeviceFolders[child.ID] != "" || rows[0].Device.ID != alpha.ID || rows[1].Folder.ID != folder.ID || rows[2].Device.ID != beta.ID || rows[3].Device.ID != child.ID {
		t.Fatal("child did not move past an unfiled device", a.Message)
	}
	a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	if a.Error || a.Layout.DeviceFolders[child.ID] != folder.ID || a.labRows()[2].Device.ID != child.ID {
		t.Fatal("moving up did not reenter the preceding folder", a.Message)
	}
	a.HandleKey(tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModAlt))
	if a.Error || a.Layout.DeviceFolders[child.ID] != "" || a.labRows()[1].Device.ID != child.ID {
		t.Fatal("moving above folder did not return to Lab", a.Message)
	}
	if err := a.Store.MoveFolder(folder.ID, 2); err == nil {
		t.Fatal("accepted invalid direction")
	}
	if err := a.Store.MoveFolder("missing", 1); err == nil {
		t.Fatal("moved missing folder")
	}
}
