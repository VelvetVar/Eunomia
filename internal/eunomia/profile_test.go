package eunomia

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestSingleFileMigratesLegacyPlacementAndExportsEverything(t *testing.T) {
	store := Store{t.TempDir()}
	first := fixtureDevice()
	first.ID, first.Name, first.CreatedAt, first.UpdatedAt = "one", "First", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z"
	second := first
	second.ID, second.Name = "two", "Second"
	legacy, err := json.Marshal(deviceFile{Version: 1, Devices: []Device{first, second}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path(), legacy, 0600); err != nil {
		t.Fatal(err)
	}
	layout := newLabLayout()
	layout.Folders = []Folder{{ID: "servers", Name: "Servers", Collapsed: true}}
	layout.Order = []string{first.ID, second.ID}
	layout.RootOrder = []string{"device:" + first.ID, "folder:servers", "device:" + second.ID}
	rawLayout, _ := json.Marshal(layout)
	if err := os.WriteFile(store.legacyLayoutPath(), rawLayout, 0600); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	profile, err := decodeDeviceFile(raw)
	if err != nil || profile.Version != 2 || profile.Layout == nil || profile.Layout.DeviceFolders[second.ID] != "servers" || !profile.Layout.Folders[0].Collapsed {
		t.Fatal("legacy placement or collapsed state lost", err)
	}
	backup, err := os.ReadFile(store.Path() + ".v1-backup")
	if err != nil || !bytes.Equal(backup, legacy) {
		t.Fatal("migration did not preserve original profiles", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(store.Path())
	if !bytes.Equal(raw, after) {
		t.Fatal("migration was not idempotent")
	}
	child, err := store.SaveFolder("Apps", "", "servers")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PlaceDevice(second.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CollapseFolder("servers", true); err != nil {
		t.Fatal(err)
	}
	export := filepath.Join(t.TempDir(), "portable.json")
	if err := store.Export(export); err != nil {
		t.Fatal(err)
	}
	if err := store.Export(export); err == nil {
		t.Fatal("export overwrote an existing file")
	}
	target := Store{t.TempDir()}
	if err := target.Import(export); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target.legacyLayoutPath()); !os.IsNotExist(err) {
		t.Fatal("import wrote a second layout file")
	}
	devices, want, err := store.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	gotDevices, got, err := target.ReadAll()
	if err != nil || !reflect.DeepEqual(devices, gotDevices) || !reflect.DeepEqual(want, got) {
		t.Fatal("single-file round trip lost profiles or hierarchy", err)
	}
	// The unified profile must stand alone, regardless of any stale sidecar.
	if err := os.WriteFile(target.legacyLayoutPath(), []byte("broken old sidecar"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := target.ReadAll(); err != nil {
		t.Fatal("unified profile still depended on lab.json", err)
	}
	second.Name = "Renamed"
	if _, err := target.Save(second, second.ID); err != nil {
		t.Fatal(err)
	}
	if layout, err := target.ReadLayout(); err != nil || layout.DeviceFolders[second.ID] != child.ID || layout.Folders[1].ParentID != "servers" {
		t.Fatal("CLI edit dropped nested organization", err)
	}
}

func TestNestedFolderUIAndAncestorCollapse(t *testing.T) {
	a, screen := uiFixture(t)
	device := saveLabDevice(t, a.Store, "Atlas")
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	typeText(a, "FServers")
	press(a, tcell.KeyEnter)
	parent, ok := a.selectedFolder()
	if !ok {
		t.Fatal("root folder not created")
	}
	typeText(a, "FApps")
	a.Draw()
	if !strings.Contains(screenText(screen), "CREATE SUBFOLDER") || !strings.Contains(screenText(screen), "Inside: Servers") {
		t.Fatal("subfolder destination is not visible")
	}
	press(a, tcell.KeyEnter)
	child, ok := a.selectedFolder()
	if !ok || child.ParentID != parent.ID {
		t.Fatal("Shift+F did not create a subfolder", a.Message)
	}
	if err := a.Store.PlaceDevice(device.ID, child.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	a.selectFolder(parent.ID)
	a.Draw()
	view := screenText(screen)
	if !strings.Contains(view, "[-] Servers (1)") || !strings.Contains(view, "\n         [-] Apps (1)") || !strings.Contains(view, "\n             Atlas") {
		t.Fatal("nested indentation or recursive counts missing", view)
	}
	press(a, tcell.KeyEnter)
	if len(a.labRows()) != 1 {
		t.Fatal("collapsing parent did not hide every descendant")
	}
	typeText(a, "/Atlas")
	press(a, tcell.KeyEnter)
	if selected, ok := a.selected(); !ok || selected.ID != device.ID {
		t.Fatal("search did not find nested device")
	}
	if a.deviceLocation(device.ID) != "Folder: Servers / Apps" {
		t.Fatal("nested path missing")
	}
	press(a, tcell.KeyEscape)
	if err := a.revealDevice(device.ID); err != nil {
		t.Fatal(err)
	}
	if selected, ok := a.selected(); !ok || selected.ID != device.ID || len(a.labRows()) != 3 {
		t.Fatal("reveal did not expand ancestors")
	}
	a.selectFolder(child.ID)
	press(a, tcell.KeyCtrlF)
	typeText(a, "Other")
	press(a, tcell.KeyEnter)
	root, ok := a.selectedFolder()
	if !ok || root.ParentID != "" {
		t.Fatal("Ctrl+F did not create a top-level folder")
	}
	if err := a.Store.DeleteFolder(parent.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.refresh(); err != nil {
		t.Fatal(err)
	}
	savedChild, err := a.Layout.folder(child.ID)
	if err != nil || savedChild.ParentID != "" || a.Layout.DeviceFolders[device.ID] != child.ID {
		t.Fatal("deleting parent lost subfolder or its devices", err)
	}
}

func TestImportRejectsBrokenHierarchyWithoutChangingProfile(t *testing.T) {
	store := Store{t.TempDir()}
	device := saveLabDevice(t, store, "Existing")
	original, _ := os.ReadFile(store.Path())
	for _, kind := range []string{"cycle", "missing-parent", "missing-device", "wrong-position"} {
		layout := newLabLayout()
		layout.Folders = []Folder{{ID: "folder", Name: "Folder"}}
		layout.RootOrder = []string{"device:" + device.ID, "folder:folder"}
		switch kind {
		case "cycle":
			layout.Folders[0].ParentID = "folder"
		case "missing-parent":
			layout.Folders[0].ParentID = "missing"
		case "missing-device":
			layout.DeviceFolders["missing"] = "folder"
		case "wrong-position":
			layout.FolderOrder["folder"] = []string{"device:" + device.ID}
		}
		raw, _ := json.Marshal(deviceFile{Version: 2, Devices: []Device{device}, Layout: &layout})
		path := filepath.Join(t.TempDir(), "bad.json")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err := store.Import(path); err == nil {
			t.Fatal("accepted invalid import", kind)
		}
		after, _ := os.ReadFile(store.Path())
		if !bytes.Equal(original, after) {
			t.Fatal("bad import replaced saved Lab", kind)
		}
	}
	if err := os.WriteFile(store.Path()+".lock", nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveFolder("Blocked", ""); err == nil {
		t.Fatal("folder write ignored shared profile lock")
	}
	after, _ := os.ReadFile(store.Path())
	if !bytes.Equal(original, after) {
		t.Fatal("locked mutation changed file")
	}
}

func TestCLIExportImport(t *testing.T) {
	t.Setenv("EUNOMIA_HOME", t.TempDir())
	store := Store{ConfigDir()}
	device := saveLabDevice(t, store, "Atlas")
	folder, err := store.SaveFolder("Servers", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PlaceDevice(device.ID, folder.ID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "lab export.json")
	var out, errOut bytes.Buffer
	if code := Main([]string{"export", path}, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	if err := store.Remove(device.ID); err != nil {
		t.Fatal(err)
	}
	if code := Main([]string{"import", path}, &out, &errOut); code == 0 {
		t.Fatal("import replaced data without --yes")
	}
	if code := Main([]string{"import", path, "--yes"}, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	devices, layout, err := store.ReadAll()
	if err != nil || len(devices) != 1 || layout.DeviceFolders[device.ID] != folder.ID {
		t.Fatal("CLI import lost folder membership", err)
	}
}
