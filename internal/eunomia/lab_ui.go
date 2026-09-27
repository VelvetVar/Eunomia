package eunomia

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
)

type labRow struct {
	Device   Device
	Folder   Folder
	IsFolder bool
	Count    int
	Depth    int
}

type folderFormState struct {
	ID, Name, ParentID string
	Cursor             int
}

type moveState struct {
	DeviceID    string
	FolderID    string
	Selected    int
	FoldersOnly bool
}

func (a *App) labRows() []labRow {
	rows := []labRow{}
	devices := map[string]Device{}
	counts := map[string]int{}
	for _, device := range a.Filtered {
		devices[device.ID] = device
		if a.Query != "" {
			rows = append(rows, labRow{Device: device})
		}
		for parent := a.Layout.DeviceFolders[device.ID]; parent != ""; {
			counts[parent]++
			folder, err := a.Layout.folder(parent)
			if err != nil {
				break
			}
			parent = folder.ParentID
		}
	}
	if a.Query != "" {
		return rows
	}
	var visit func(string, int)
	visit = func(parent string, depth int) {
		for _, entry := range a.Layout.items(parent, a.Filtered) {
			kind, id, _ := strings.Cut(entry, ":")
			if kind == "device" {
				rows = append(rows, labRow{Device: devices[id], Depth: depth})
				continue
			}
			folder, err := a.Layout.folder(id)
			if err != nil {
				continue
			}
			rows = append(rows, labRow{Folder: *folder, IsFolder: true, Count: counts[id], Depth: depth})
			if !folder.Collapsed {
				visit(id, depth+1)
			}
		}
	}
	visit("", 0)
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
		collapsed := false
		for current := folderID; current != ""; {
			folder, err := a.Layout.folder(current)
			if err != nil {
				break
			}
			collapsed = collapsed || folder.Collapsed
			current = folder.ParentID
		}
		if collapsed {
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
	previousFolder := a.Layout.DeviceFolders[id]
	err := a.Store.MoveDevice(id, direction)
	if err == nil {
		a.Query = ""
		err = a.refresh()
		if err == nil {
			err = a.revealDevice(id)
		}
	}
	message := "Device order saved."
	if err == nil && a.Layout.DeviceFolders[id] != previousFolder {
		message = "Moved to " + a.deviceLocation(id) + "."
	}
	a.setMessage(err, message)
}

func (a *App) moveFolder(id string, direction int) {
	err := a.Store.MoveFolder(id, direction)
	if err == nil {
		a.Query = ""
		err = a.refresh()
		a.selectFolder(id)
	}
	a.setMessage(err, "Folder moved with its devices.")
}

func (a *App) beginFolder(folder Folder) {
	a.Mode, a.Message, a.Error = "folder", "", false
	a.FolderForm = folderFormState{ID: folder.ID, Name: folder.Name, ParentID: folder.ParentID, Cursor: len([]rune(folder.Name))}
}

func (a *App) deviceLocation(id string) string {
	if path := a.Layout.folderPath(a.Layout.DeviceFolders[id]); path != "" {
		return "Folder: " + path
	}
	return "Lab (no folder)"
}

func (a *App) beginMove(id string, foldersOnly bool) {
	a.Mode, a.Message, a.Error = "move", "", false
	a.Move = moveState{DeviceID: id, FoldersOnly: foldersOnly}
	if foldersOnly {
		a.Move.Selected = 2
		for i, folder := range a.Layout.Folders {
			if i == 0 || folder.ID == a.Layout.DeviceFolders[id] {
				a.Move.Selected = i + 3
			}
		}
	}
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
		folder, err := a.Store.SaveFolder(a.FolderForm.Name, a.FolderForm.ID, a.FolderForm.ParentID)
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
		first := 0
		if a.Move.FoldersOnly {
			first = 2
		}
		a.Move.Selected = max(first, a.Move.Selected-1)
	case event.Key() == tcell.KeyDown || event.Rune() == 'j':
		last := len(a.Layout.Folders) + 2
		if a.Move.FolderID != "" {
			last = 1
		}
		a.Move.Selected = min(last, a.Move.Selected+1)
	case event.Key() == tcell.KeyEnter:
		if a.Move.Selected < 2 {
			a.Mode = "list"
			if a.Move.FolderID != "" {
				a.moveFolder(a.Move.FolderID, 2*a.Move.Selected-1)
			} else {
				a.moveDevice(a.Move.DeviceID, 2*a.Move.Selected-1)
			}
			return true
		}
		folderID, destination := "", "Lab (no folder)"
		if a.Move.Selected >= 3 {
			if a.Move.Selected-3 >= len(a.Layout.Folders) {
				return true
			}
			folderID = a.Layout.Folders[a.Move.Selected-3].ID
			destination = "folder: " + a.Layout.folderPath(folderID)
		}
		err := a.Store.PlaceDevice(a.Move.DeviceID, folderID)
		if err == nil {
			a.Mode, a.Query = "list", ""
			err = a.refresh()
			a.selectDevice(a.Move.DeviceID)
		}
		a.setMessage(err, "Device moved to "+destination+".")
	}
	return true
}

func (a *App) drawFolderForm(w, top int) {
	title := "CREATE FOLDER"
	if a.FolderForm.ParentID != "" {
		title = "CREATE SUBFOLDER"
	}
	if a.FolderForm.ID != "" {
		title = "RENAME FOLDER"
	}
	a.put(3, top+1, title, tealStyle, w-6)
	if a.FolderForm.ParentID != "" {
		a.put(3, top+2, "Inside: "+a.Layout.folderPath(a.FolderForm.ParentID), dimStyle, w-6)
	}
	a.put(3, top+3, "Folder name", dimStyle, w-6)
	a.put(3, top+5, inputView(a.FolderForm.Name, a.FolderForm.Cursor, w-6), whiteStyle, w-6)
	a.put(3, top+7, "Use Shift+M on a device to move it into a folder.", dimStyle, w-6)
}

func (a *App) drawMove(w, bottom, top int) {
	title := "MOVE DEVICE"
	if a.Move.FoldersOnly {
		title = "MOVE TO FOLDER"
	} else if a.Move.FolderID != "" {
		title = "MOVE FOLDER"
	}
	a.put(3, top+1, title, tealStyle, w-6)
	if device, err := Find(a.Devices, a.Move.DeviceID); err == nil {
		a.put(3, top+2, device.Name, whiteStyle, w-6)
	}
	if a.Move.FolderID != "" {
		if folder, err := a.Layout.folder(a.Move.FolderID); err == nil {
			a.put(3, top+2, folder.Name, whiteStyle, w-6)
		}
		a.put(3, top+3, "The folder and all its devices move together.", dimStyle, w-6)
		a.put(3, top+4, "Systems below the heading join this folder.", dimStyle, w-6)
		for i, label := range []string{"Move folder up", "Move folder down"} {
			style, marker := baseStyle, "  "
			if i == a.Move.Selected {
				style, marker = tealStyle, "› "
			}
			a.put(3, top+5+i, marker+label, style, w-6)
		}
		return
	}
	a.put(3, top+3, "Current location: "+a.deviceLocation(a.Move.DeviceID), dimStyle, w-6)
	choices := []string{"Move up in Lab", "Move down in Lab", "Move to Lab (no folder)"}
	for _, folder := range a.Layout.Folders {
		choices = append(choices, "Move to folder: "+a.Layout.folderPath(folder.ID))
	}
	first := 0
	if a.Move.FoldersOnly {
		first = 2
		if len(a.Layout.Folders) == 0 {
			a.put(3, top+4, "No folders yet. Esc, then Shift+F to create one.", dimStyle, w-6)
		}
	} else {
		a.put(3, top+4, "Crossing a folder heading changes membership.", dimStyle, w-6)
	}
	visible := max(1, bottom-top-5)
	offset := max(first, a.Move.Selected-visible+1)
	for i := offset; i < min(len(choices), offset+visible); i++ {
		style, marker := baseStyle, "  "
		if i == a.Move.Selected {
			style, marker = tealStyle, "› "
		}
		a.put(3, top+5+i-offset, marker+choices[i], style, w-6)
	}
}

func (a *App) folderSummary(row labRow) string {
	state := "expanded"
	if row.Folder.Collapsed {
		state = "collapsed"
	}
	return fmt.Sprintf("%d devices / %s / Enter toggles", row.Count, state)
}
