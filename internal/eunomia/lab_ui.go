package eunomia

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
)

type labRow struct {
	Device   Device
	Folder   Folder
	IsFolder bool
	Count    int
}

type folderFormState struct {
	ID, Name string
	Cursor   int
}

type moveState struct {
	DeviceID string
	Selected int
}

func (a *App) labRows() []labRow {
	rows := []labRow{}
	groups := map[string][]Device{}
	for _, device := range a.Filtered {
		folder := a.Layout.DeviceFolders[device.ID]
		// Search always finds devices inside collapsed folders.
		if folder == "" || a.Query != "" {
			rows = append(rows, labRow{Device: device})
		} else {
			groups[folder] = append(groups[folder], device)
		}
	}
	if a.Query != "" {
		return rows
	}
	for _, folder := range a.Layout.Folders {
		devices := groups[folder.ID]
		rows = append(rows, labRow{Folder: folder, IsFolder: true, Count: len(devices)})
		if !folder.Collapsed {
			for _, device := range devices {
				rows = append(rows, labRow{Device: device})
			}
		}
	}
	return rows
}

func (a *App) selectedFolder() (Folder, bool) {
	rows := a.labRows()
	if a.Selected >= 0 && a.Selected < len(rows) && rows[a.Selected].IsFolder {
		return rows[a.Selected].Folder, true
	}
	return Folder{}, false
}

func (a *App) selectFolder(id string) {
	for i, row := range a.labRows() {
		if row.IsFolder && row.Folder.ID == id {
			a.Selected = i
			return
		}
	}
}

func (a *App) selectDevice(id string) {
	for i, row := range a.labRows() {
		if !row.IsFolder && row.Device.ID == id {
			a.Selected = i
			return
		}
	}
}

func (a *App) revealDevice(id string) error {
	if folderID := a.Layout.DeviceFolders[id]; folderID != "" && a.Query == "" {
		if folder, err := a.Layout.folder(folderID); err == nil && folder.Collapsed {
			if err := a.Store.CollapseFolder(folderID, false); err != nil {
				return err
			}
			if err := a.refresh(); err != nil {
				return err
			}
		}
	}
	a.selectDevice(id)
	return nil
}

func (a *App) collapseFolder(folder Folder, collapsed bool) {
	err := a.Store.CollapseFolder(folder.ID, collapsed)
	if err == nil {
		err = a.refresh()
		a.selectFolder(folder.ID)
	}
	a.setMessage(err, "")
}

func (a *App) moveDevice(id string, direction int) {
	err := a.Store.MoveDevice(id, direction)
	if err == nil {
		a.Query = ""
		err = a.refresh()
		if err == nil {
			err = a.revealDevice(id)
		}
	}
	a.setMessage(err, "Device order saved.")
}

func (a *App) beginFolder(folder Folder) {
	a.Mode, a.Message, a.Error = "folder", "", false
	a.FolderForm = folderFormState{ID: folder.ID, Name: folder.Name, Cursor: len([]rune(folder.Name))}
}

func (a *App) labModeKey(event *tcell.EventKey) bool {
	if a.Mode != "folder" && a.Mode != "move" {
		return false
	}
	if event.Key() == tcell.KeyEscape {
		a.Mode, a.Message, a.Error = "list", "", false
		return true
	}
	if a.Mode == "folder" {
		if event.Key() != tcell.KeyEnter {
			editText(&a.FolderForm.Name, &a.FolderForm.Cursor, event, 60)
			return true
		}
		folder, err := a.Store.SaveFolder(a.FolderForm.Name, a.FolderForm.ID)
		if err == nil {
			a.Mode, a.Query = "list", ""
			err = a.refresh()
			a.selectFolder(folder.ID)
		}
		a.setMessage(err, "Saved folder "+folder.Name+".")
		return true
	}
	switch {
	case event.Key() == tcell.KeyUp || event.Rune() == 'k':
		a.Move.Selected = max(0, a.Move.Selected-1)
	case event.Key() == tcell.KeyDown || event.Rune() == 'j':
		a.Move.Selected = min(len(a.Layout.Folders)+2, a.Move.Selected+1)
	case event.Key() == tcell.KeyEnter:
		if a.Move.Selected < 2 {
			a.Mode = "list"
			a.moveDevice(a.Move.DeviceID, 2*a.Move.Selected-1)
			return true
		}
		folderID := ""
		if a.Move.Selected >= 3 {
			if a.Move.Selected-3 >= len(a.Layout.Folders) {
				return true
			}
			folderID = a.Layout.Folders[a.Move.Selected-3].ID
		}
		err := a.Store.PlaceDevice(a.Move.DeviceID, folderID)
		if err == nil {
			a.Mode, a.Query = "list", ""
			err = a.refresh()
			a.selectDevice(a.Move.DeviceID)
		}
		a.setMessage(err, "Device moved.")
	}
	return true
}

func (a *App) drawFolderForm(w, top int) {
	title := "CREATE FOLDER"
	if a.FolderForm.ID != "" {
		title = "RENAME FOLDER"
	}
	a.put(3, top+1, title, tealStyle, w-6)
	a.put(3, top+3, "Folder name", dimStyle, w-6)
	a.put(3, top+5, inputView(a.FolderForm.Name, a.FolderForm.Cursor, w-6), whiteStyle, w-6)
	a.put(3, top+7, "Use M on a device to move it into a folder.", dimStyle, w-6)
}

func (a *App) drawMove(w, h, top int) {
	a.put(3, top+1, "MOVE DEVICE", tealStyle, w-6)
	if device, err := Find(a.Devices, a.Move.DeviceID); err == nil {
		a.put(3, top+2, device.Name, whiteStyle, w-6)
	}
	choices := []string{"Move up in this group", "Move down in this group", "Move to Lab (no folder)"}
	for _, folder := range a.Layout.Folders {
		choices = append(choices, "Move to folder: "+folder.Name)
	}
	visible := max(1, h-top-8)
	offset := max(0, a.Move.Selected-visible+1)
	for i := offset; i < min(len(choices), offset+visible); i++ {
		style, marker := baseStyle, "  "
		if i == a.Move.Selected {
			style, marker = tealStyle, "› "
		}
		a.put(3, top+4+i-offset, marker+choices[i], style, w-6)
	}
}

func (a *App) folderSummary(row labRow) string {
	state := "expanded"
	if row.Folder.Collapsed {
		state = "collapsed"
	}
	return fmt.Sprintf("%d devices / %s / Enter toggles", row.Count, state)
}
