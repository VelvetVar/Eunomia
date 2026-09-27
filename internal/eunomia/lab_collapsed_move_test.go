package eunomia

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestMovingPastCollapsedFoldersPreservesMembership(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, shortcut := range []bool{false, true} {
			t.Run(fmt.Sprintf("nested=%t/shortcut=%t", nested, shortcut), func(t *testing.T) {
				a, _ := uiFixture(t)
				mover := saveLabDevice(t, a.Store, "Mover")
				hidden := saveLabDevice(t, a.Store, "Hidden member")
				parent := ""
				if nested {
					outer, err := a.Store.SaveFolder("Outer", "")
					if err != nil {
						t.Fatal(err)
					}
					parent = outer.ID
					if err := a.Store.PlaceDevice(mover.ID, parent); err != nil {
						t.Fatal(err)
					}
				}
				closed, err := a.Store.SaveFolder("Closed", "", parent)
				if err != nil {
					t.Fatal(err)
				}
				child, err := a.Store.SaveFolder("Hidden subfolder", "", closed.ID)
				if err != nil {
					t.Fatal(err)
				}
				if err := a.Store.PlaceDevice(hidden.ID, child.ID); err != nil {
					t.Fatal(err)
				}
				if err := a.Store.CollapseFolder(closed.ID, true); err != nil {
					t.Fatal(err)
				}
				if err := a.refresh(); err != nil {
					t.Fatal(err)
				}
				move := func(down bool) {
					t.Helper()
					if shortcut {
						key := tcell.KeyUp
						if down {
							key = tcell.KeyDown
						}
						a.HandleKey(tcell.NewEventKey(key, 0, tcell.ModAlt))
					} else {
						typeText(a, "M")
						if down {
							press(a, tcell.KeyDown)
						}
						press(a, tcell.KeyEnter)
					}
					if a.Error {
						t.Fatal(a.Message)
					}
				}
				checkClosed := func(folderFirst bool) {
					t.Helper()
					layout, err := (Store{a.Store.Directory}).ReadLayout()
					if err != nil {
						t.Fatal(err)
					}
					folder, _ := layout.folder(closed.ID)
					if !folder.Collapsed || layout.DeviceFolders[mover.ID] != parent || layout.DeviceFolders[hidden.ID] != child.ID {
						t.Fatal("passing a closed folder changed its state or membership", layout)
					}
					want := []string{"device:" + mover.ID, "folder:" + closed.ID}
					if folderFirst {
						want[0], want[1] = want[1], want[0]
					}
					if !reflect.DeepEqual(layout.storedItems(parent), want) {
						t.Fatal("did not pass the collapsed heading", layout.storedItems(parent), want)
					}
					rows := a.labRows()
					if nested {
						rows = rows[1:]
					}
					if len(rows) != 2 || rows[0].Depth != rows[1].Depth {
						t.Fatal("passing device became hidden or indented inside the closed folder", rows)
					}
				}
				a.selectDevice(mover.ID)
				move(true)
				checkClosed(true)
				if selected, ok := a.selected(); !ok || selected.ID != mover.ID {
					t.Fatal("movement lost device selection")
				}
				// Expansion, unrelated writes and export/import must not later adopt it.
				if err := a.Store.CollapseFolder(closed.ID, false); err != nil {
					t.Fatal(err)
				}
				hidden.Description = "Edited after the move"
				if _, err := a.Store.Save(hidden, hidden.ID); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(t.TempDir(), "lab.json")
				if err := a.Store.Export(path); err != nil {
					t.Fatal(err)
				}
				target := Store{t.TempDir()}
				if err := target.Import(path); err != nil {
					t.Fatal(err)
				}
				layout, err := target.ReadLayout()
				if err != nil || layout.DeviceFolders[mover.ID] != parent || layout.DeviceFolders[hidden.ID] != child.ID {
					t.Fatal("expansion or export/import adopted a device that passed a closed folder", err)
				}
				if err := a.Store.CollapseFolder(closed.ID, true); err != nil {
					t.Fatal(err)
				}
				if err := a.refresh(); err != nil {
					t.Fatal(err)
				}
				a.selectDevice(mover.ID)
				move(false)
				checkClosed(false)
				// Moving the closed folder itself past the system also ignores it.
				a.selectFolder(closed.ID)
				move(false)
				checkClosed(true)
				move(true)
				checkClosed(false)
			})
		}
	}
}

func TestMovingOutOfFolderPastClosedSibling(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprintf("nested=%t", nested), func(t *testing.T) {
			store := Store{t.TempDir()}
			mover := saveLabDevice(t, store, "Mover")
			parent := ""
			if nested {
				outer, err := store.SaveFolder("Outer", "")
				if err != nil {
					t.Fatal(err)
				}
				parent = outer.ID
			}
			folders := []Folder{}
			for _, name := range []string{"Source", "Closed", "Destination"} {
				folder, err := store.SaveFolder(name, "", parent)
				if err != nil {
					t.Fatal(err)
				}
				folders = append(folders, folder)
			}
			if err := store.PlaceDevice(mover.ID, folders[0].ID); err != nil {
				t.Fatal(err)
			}
			if err := store.CollapseFolder(folders[1].ID, true); err != nil {
				t.Fatal(err)
			}
			for _, step := range []struct {
				direction int
				parent    string
			}{{1, parent}, {1, folders[2].ID}, {-1, parent}, {-1, folders[0].ID}} {
				if err := store.MoveDevice(mover.ID, step.direction); err != nil {
					t.Fatal(err)
				}
				layout, err := store.ReadLayout()
				if err != nil || layout.DeviceFolders[mover.ID] != step.parent || !layout.Folders[len(layout.Folders)-2].Collapsed {
					t.Fatal("crossing a closed sibling entered or expanded it", step, layout, err)
				}
			}
		})
	}
}

func TestMovingPastUnfiledDeviceAfterReopeningFolder(t *testing.T) {
	store := Store{t.TempDir()}
	mover := saveLabDevice(t, store, "Member")
	outside := saveLabDevice(t, store, "Outside")
	folder, err := store.SaveFolder("Folder", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{
		store.PlaceDevice(mover.ID, folder.ID),
		store.CollapseFolder(folder.ID, true),
		store.MoveDevice(outside.ID, 1),
		store.CollapseFolder(folder.ID, false),
		store.MoveDevice(mover.ID, 1),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	layout, err := store.ReadLayout()
	want := []string{"folder:" + folder.ID, "device:" + outside.ID, "device:" + mover.ID}
	if err != nil || len(layout.DeviceFolders) != 0 || !reflect.DeepEqual(layout.RootOrder, want) {
		t.Fatal("moving past an outside device re-adopted the mover", layout, err)
	}
}
