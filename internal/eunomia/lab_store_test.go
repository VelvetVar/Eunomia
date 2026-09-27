package eunomia

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func savedProfileBytes(t *testing.T, store Store) []byte {
	t.Helper()
	devices, err := store.Read()
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(devices)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func saveLabDevice(t *testing.T, store Store, name string) Device {
	t.Helper()
	device := fixtureDevice()
	device.Name = name
	saved, err := store.Save(device, "")
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func layoutIDs(layout LabLayout, devices []Device, folder string) []string {
	var ids []string
	for _, device := range layout.search(devices, "") {
		if layout.DeviceFolders[device.ID] == folder {
			ids = append(ids, device.ID)
		}
	}
	return ids
}

func TestLabOrganizationPersistsWithoutChangingProfiles(t *testing.T) {
	store := Store{t.TempDir()}
	a := saveLabDevice(t, store, "A")
	b := saveLabDevice(t, store, "B")
	c := saveLabDevice(t, store, "C")
	original := savedProfileBytes(t, store)
	layout, err := store.ReadLayout()
	if err != nil || len(layout.Folders) != 0 || len(layout.Order) != 3 {
		t.Fatal("profiles were not saved together with their order", err)
	}
	if err := store.MoveDevice(c.ID, -1); err != nil {
		t.Fatal(err)
	}
	devices, _ := store.Read()
	layout, _ = store.ReadLayout()
	if !reflect.DeepEqual(layoutIDs(layout, devices, ""), []string{a.ID, c.ID, b.ID}) {
		t.Fatal("move did not persist relative order")
	}
	folder, err := store.SaveFolder("  Servers  ", "")
	if err != nil || folder.Name != "Servers" {
		t.Fatal(folder, err)
	}
	for _, id := range []string{c.ID, a.ID} {
		if err := store.PlaceDevice(id, folder.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.MoveDevice(a.ID, -1); err != nil {
		t.Fatal(err)
	}
	if err := store.CollapseFolder(folder.ID, true); err != nil {
		t.Fatal(err)
	}
	layout, err = (Store{store.Directory}).ReadLayout()
	if err != nil || !layout.Folders[0].Collapsed || !reflect.DeepEqual(layoutIDs(layout, devices, folder.ID), []string{a.ID, c.ID}) {
		t.Fatal("folder state or order lost on reload", err)
	}
	if !reflect.DeepEqual(layoutIDs(layout, devices, ""), []string{b.ID}) {
		t.Fatal("moving inside a folder reordered or moved an unrelated device")
	}
	after := savedProfileBytes(t, store)
	if !bytes.Equal(original, after) {
		t.Fatal("organization rewrote device profiles")
	}
	if err := store.DeleteFolder(folder.ID); err != nil {
		t.Fatal(err)
	}
	layout, _ = store.ReadLayout()
	after = savedProfileBytes(t, store)
	if len(layout.Folders) != 0 || len(layout.DeviceFolders) != 0 || !bytes.Equal(original, after) {
		t.Fatal("deleting a folder deleted or changed devices")
	}
}

func TestLabLayoutSurvivesProfileEditsAndPrunesRemovedDevices(t *testing.T) {
	store := Store{t.TempDir()}
	a := saveLabDevice(t, store, "A")
	b := saveLabDevice(t, store, "B")
	folder, _ := store.SaveFolder("Servers", "")
	if err := store.PlaceDevice(a.ID, folder.ID); err != nil {
		t.Fatal(err)
	}
	a.Name = "Renamed"
	if _, err := store.Save(a, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Remove(b.ID); err != nil {
		t.Fatal(err)
	}
	c := saveLabDevice(t, store, "C")
	if err := store.CollapseFolder(folder.ID, true); err != nil {
		t.Fatal(err)
	}
	layout, err := store.ReadLayout()
	if err != nil || layout.DeviceFolders[a.ID] != folder.ID || !reflect.DeepEqual(layout.Order, []string{c.ID, a.ID}) {
		t.Fatal("CLI profile changes lost folder membership or left stale order", layout, err)
	}
	if err := store.PlaceDevice(c.ID, folder.ID); err != nil {
		t.Fatal(err)
	}
	layout, _ = store.ReadLayout()
	if layout.Folders[0].Collapsed {
		t.Fatal("moving into a folder did not expand it")
	}
	if err := store.PlaceDevice(a.ID, ""); err != nil {
		t.Fatal(err)
	}
	layout, _ = store.ReadLayout()
	if layout.DeviceFolders[a.ID] != "" || layout.DeviceFolders[c.ID] != folder.ID {
		t.Fatal("move back to Lab changed other membership")
	}
}

func TestLabMoveThroughFolderBoundaries(t *testing.T) {
	store := Store{t.TempDir()}
	devices := []Device{}
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		devices = append(devices, saveLabDevice(t, store, name))
	}
	folders := []Folder{}
	for _, name := range []string{"Servers", "Empty", "Network"} {
		folder, err := store.SaveFolder(name, "")
		if err != nil {
			t.Fatal(err)
		}
		folders = append(folders, folder)
	}
	for i, folder := range []string{"", "", folders[0].ID, folders[0].ID, folders[2].ID} {
		if folder == "" {
			continue
		}
		if err := store.PlaceDevice(devices[i].ID, folder); err != nil {
			t.Fatal(err)
		}
	}
	original := savedProfileBytes(t, store)
	for _, folder := range folders {
		if err := store.CollapseFolder(folder.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	steps := []struct {
		direction int
		groups    [4]string // Lab, Servers, Empty, Network, in display order.
		edge      bool
	}{
		{1, [4]string{"A", "BCD", "", "E"}, false},
		{1, [4]string{"A", "CBD", "", "E"}, false},
		{1, [4]string{"A", "CDB", "", "E"}, false},
		{1, [4]string{"A", "CD", "B", "E"}, false},
		{1, [4]string{"A", "CD", "", "BE"}, false},
		{1, [4]string{"A", "CD", "", "EB"}, false},
		{1, [4]string{"A", "CD", "", "EB"}, true},
		{-1, [4]string{"A", "CD", "", "BE"}, false},
		{-1, [4]string{"A", "CD", "B", "E"}, false},
		{-1, [4]string{"A", "CDB", "", "E"}, false},
		{-1, [4]string{"A", "CBD", "", "E"}, false},
		{-1, [4]string{"A", "BCD", "", "E"}, false},
		{-1, [4]string{"AB", "CD", "", "E"}, false},
		{-1, [4]string{"BA", "CD", "", "E"}, false},
		{-1, [4]string{"BA", "CD", "", "E"}, true},
	}
	for i, step := range steps {
		// Also exercise entering collapsed folders while moving up.
		if i == 8 || i == 9 {
			if err := store.CollapseFolder(folders[9-i].ID, true); err != nil {
				t.Fatal(err)
			}
		}
		before, _ := os.ReadFile(store.LayoutPath())
		err := store.MoveDevice(devices[1].ID, step.direction)
		if (err != nil) != step.edge {
			t.Fatalf("step %d: unexpected edge result: %v", i, err)
		}
		layout, err := (Store{store.Directory}).ReadLayout()
		if err != nil {
			t.Fatal(err)
		}
		var got [4]string
		for j, folder := range []string{"", folders[0].ID, folders[1].ID, folders[2].ID} {
			for _, id := range layoutIDs(layout, devices, folder) {
				device, err := Find(devices, id)
				if err != nil {
					t.Fatal(err)
				}
				got[j] += device.Name
			}
		}
		if got != step.groups {
			t.Fatalf("step %d: groups %v, want %v", i, got, step.groups)
		}
		if folderID := layout.DeviceFolders[devices[1].ID]; folderID != "" {
			folder, err := layout.folder(folderID)
			if err != nil || folder.Collapsed {
				t.Fatalf("step %d: destination was not expanded", i)
			}
		}
		if step.edge {
			after, _ := os.ReadFile(store.LayoutPath())
			if !bytes.Equal(before, after) {
				t.Fatal("edge error changed the saved layout")
			}
		}
	}
	after := savedProfileBytes(t, store)
	if !bytes.Equal(original, after) {
		t.Fatal("moving across folders changed device profiles")
	}
}

func TestLabLayoutValidationAndLockPreserveOriginalFiles(t *testing.T) {
	store := Store{t.TempDir()}
	device := saveLabDevice(t, store, "A")
	profiles := savedProfileBytes(t, store)
	for _, original := range []string{
		`{"version":9,"folders":[],"order":[],"deviceFolders":{}}`,
		`{"version":1,"folders":[{"id":"one","name":"A"},{"id":"two","name":" a "}],"order":[],"deviceFolders":{}}`,
		`{"version":1,"folders":[],"order":["same","same"],"deviceFolders":{}}`,
		`{"version":1,"folders":[],"order":[],"deviceFolders":{"device":"missing"}}`,
		`{"version":1,"folders":[],"order":[],"deviceFolders":{},"rootOrder":["wrong:id"]}`,
		`{"version":1,"folders":[],"order":[],"deviceFolders":{},"rootOrder":["folder:one","folder:one"]}`,
		`{"version":1}`, `{broken`,
	} {
		original = `{"version":2,"devices":` + string(profiles) + `,"lab":` + original + `}`
		if err := os.WriteFile(store.LayoutPath(), []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.ReadLayout(); err == nil {
			t.Fatal("accepted invalid layout", original)
		}
		if _, err := store.SaveFolder("Replacement", ""); err == nil {
			t.Fatal("overwrote invalid layout")
		}
		data, _ := os.ReadFile(store.LayoutPath())
		if string(data) != original {
			t.Fatal("invalid layout changed")
		}
		if _, err := store.Read(); err == nil {
			t.Fatal("accepted corrupt organization in a unified profile", device.ID)
		}
	}
	os.Remove(store.LayoutPath())
	os.WriteFile(store.LayoutPath()+".lock", nil, 0600)
	if _, err := store.SaveFolder("Blocked", ""); err == nil {
		t.Fatal("ignored layout writer lock")
	}
	if _, err := os.Stat(store.LayoutPath()); !os.IsNotExist(err) {
		t.Fatal("wrote a layout despite lock")
	}
}

func TestLabFolderNamesAndFailedMoves(t *testing.T) {
	store := Store{t.TempDir()}
	device := saveLabDevice(t, store, "A")
	folder, err := store.SaveFolder("Servers", "")
	if err != nil {
		t.Fatal(err)
	}
	original, _ := os.ReadFile(store.LayoutPath())
	for _, name := range []string{"", "  ", "servers", "bad\nname", "bad\x1bname"} {
		if _, err := store.SaveFolder(name, ""); err == nil {
			t.Fatal("accepted invalid or duplicate folder name")
		}
	}
	for _, err := range []error{store.PlaceDevice(device.ID, "missing"), store.PlaceDevice("missing", folder.ID), store.MoveDevice(device.ID, -1), store.MoveDevice(device.ID, 2), store.DeleteFolder("missing")} {
		if err == nil {
			t.Fatal("invalid organization action succeeded")
		}
	}
	after, _ := os.ReadFile(store.LayoutPath())
	if !bytes.Equal(original, after) {
		t.Fatal("failed action changed the layout")
	}
	renamed, err := store.SaveFolder("Network", folder.ID)
	if err != nil || renamed.ID != folder.ID || renamed.Name != "Network" {
		t.Fatal("rename changed folder identity", err)
	}
}
