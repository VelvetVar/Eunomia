package eunomia

import (
	"errors"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

type Folder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parentId,omitempty"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

type LabLayout struct {
	Version       int                 `json:"version"`
	Folders       []Folder            `json:"folders"`
	Order         []string            `json:"order"`
	RootOrder     []string            `json:"rootOrder,omitempty"`
	FolderOrder   map[string][]string `json:"folderOrder,omitempty"`
	DeviceFolders map[string]string   `json:"deviceFolders"`
}

func newLabLayout() LabLayout {
	return LabLayout{Version: 1, Folders: []Folder{}, Order: []string{}, DeviceFolders: map[string]string{}, FolderOrder: map[string][]string{}}
}

func folderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 60 || safe(name) != name {
		return "", errors.New("folder name must be 1–60 characters without control characters")
	}
	return name, nil
}

func (l *LabLayout) validate() error {
	if l.Version != 1 || l.Folders == nil || l.Order == nil || l.DeviceFolders == nil {
		return errors.New("invalid Lab layout; original file left untouched")
	}
	validID := func(id string) bool { return id != "" && len(id) <= 256 && safe(id) == id && utf8.ValidString(id) }
	ids, names := map[string]bool{}, map[string]bool{}
	for i, folder := range l.Folders {
		name, err := folderName(folder.Name)
		key := folder.ParentID + "\x00" + strings.ToLower(name)
		if err != nil || !validID(folder.ID) || ids[folder.ID] || names[key] {
			return errors.New("invalid or duplicate folder")
		}
		ids[folder.ID], names[key] = true, true
		l.Folders[i].Name = name
	}
	for _, folder := range l.Folders {
		seen := map[string]bool{folder.ID: true}
		parent := folder.ParentID
		for parent != "" {
			if !ids[parent] || seen[parent] || len(seen) >= 32 {
				return errors.New("invalid folder parent, cycle, or nesting deeper than 32 levels")
			}
			seen[parent] = true
			f, _ := l.folder(parent)
			parent = f.ParentID
		}
	}
	seen := map[string]bool{}
	for _, id := range l.Order {
		if !validID(id) || seen[id] {
			return errors.New("invalid device order")
		}
		seen[id] = true
	}
	for device, folder := range l.DeviceFolders {
		if !validID(device) || !ids[folder] {
			return errors.New("invalid folder assignment")
		}
	}
	orders := [][]string{l.RootOrder}
	for id, order := range l.FolderOrder {
		if !ids[id] {
			return errors.New("order refers to a missing folder")
		}
		orders = append(orders, order)
	}
	for _, order := range orders {
		seen = map[string]bool{}
		for _, entry := range order {
			kind, id, _ := strings.Cut(entry, ":")
			if (kind != "device" && kind != "folder") || !validID(id) || seen[entry] {
				return errors.New("invalid Lab order")
			}
			seen[entry] = true
		}
	}
	return nil
}

func (l *LabLayout) folder(id string) (*Folder, error) {
	for i := range l.Folders {
		if l.Folders[i].ID == id {
			return &l.Folders[i], nil
		}
	}
	return nil, errors.New("folder no longer exists; reload Lab")
}

func (l LabLayout) folderPath(id string) string {
	names := []string{}
	for id != "" {
		folder, err := l.folder(id)
		if err != nil {
			break
		}
		names = append(names, folder.Name)
		id = folder.ParentID
	}
	slices.Reverse(names)
	return strings.Join(names, " / ")
}

func (l *LabLayout) expandAncestors(id string) {
	for id != "" {
		f, err := l.folder(id)
		if err != nil {
			return
		}
		f.Collapsed = false
		id = f.ParentID
	}
}

func (l LabLayout) search(devices []Device, query string) []Device {
	result := Search(devices, query)
	ranks := map[string]int{}
	for i, id := range l.Order {
		ranks[id] = i
	}
	rank := func(id string) int {
		if n, ok := ranks[id]; ok {
			return n
		}
		return len(l.Order)
	}
	sort.SliceStable(result, func(i, j int) bool { return rank(result[i].ID) < rank(result[j].ID) })
	return result
}

func (l LabLayout) storedItems(parent string) []string {
	if parent == "" {
		return l.RootOrder
	}
	return l.FolderOrder[parent]
}
func (l *LabLayout) setItems(parent string, items []string) {
	if parent == "" {
		l.RootOrder = items
	} else {
		if l.FolderOrder == nil {
			l.FolderOrder = map[string][]string{}
		}
		l.FolderOrder[parent] = items
	}
}

func (l LabLayout) items(parent string, devices []Device) []string {
	valid := map[string]bool{}
	for _, device := range devices {
		if l.DeviceFolders[device.ID] == parent {
			valid["device:"+device.ID] = true
		}
	}
	for _, folder := range l.Folders {
		if folder.ParentID == parent {
			valid["folder:"+folder.ID] = true
		}
	}
	items := []string{}
	for _, entry := range l.storedItems(parent) {
		if valid[entry] {
			items = append(items, entry)
			delete(valid, entry)
		}
	}
	for _, device := range l.search(devices, "") {
		entry := "device:" + device.ID
		if valid[entry] {
			position := 0
			for i, item := range items {
				if strings.HasPrefix(item, "device:") {
					position = i + 1
				}
			}
			items = slices.Insert(items, position, entry)
			delete(valid, entry)
		}
	}
	for _, folder := range l.Folders {
		entry := "folder:" + folder.ID
		if valid[entry] {
			items = append(items, entry)
		}
	}
	return items
}

func (l LabLayout) rootOrder(devices []Device) []string { return l.items("", devices) }

// Reconcile saved order without changing membership on reload or expansion.
// Only devices affected by a move may join an expanded heading they pass.
func (l *LabLayout) normalize(devices []Device, adoptIDs ...string) {
	adopt := map[string]bool{}
	for _, id := range adoptIDs {
		adopt[id] = true
	}
	known := map[string]bool{}
	for _, device := range devices {
		known[device.ID] = true
	}
	for id := range l.DeviceFolders {
		if !known[id] {
			delete(l.DeviceFolders, id)
		}
	}
	groups := map[string][]string{"": l.items("", devices)}
	for _, folder := range l.Folders {
		groups[folder.ID] = l.items(folder.ID, devices)
	}
	l.FolderOrder = map[string][]string{}
	l.Order = []string{}
	var visit func(string)
	visit = func(parent string) {
		items := []string{}
		previousFolder := ""
		for _, entry := range groups[parent] {
			kind, id, _ := strings.Cut(entry, ":")
			if kind == "folder" {
				previousFolder = id
				folder, _ := l.folder(id)
				if folder.Collapsed {
					previousFolder = ""
				}
				items = append(items, entry)
			} else if previousFolder != "" && adopt[id] {
				l.DeviceFolders[id] = previousFolder
				groups[previousFolder] = append(groups[previousFolder], entry)
			} else {
				items = append(items, entry)
			}
		}
		l.setItems(parent, items)
		for _, entry := range items {
			kind, id, _ := strings.Cut(entry, ":")
			if kind == "folder" {
				visit(id)
			} else {
				l.Order = append(l.Order, id)
			}
		}
	}
	visit("")
}

func (s Store) mutateLayout(change func(*LabLayout, []Device) error) error {
	return s.mutateData(func(data *deviceFile) error { return change(data.Layout, data.Devices) })
}

func (s Store) SaveFolder(name, id string, parentIDs ...string) (Folder, error) {
	var saved Folder
	err := s.mutateLayout(func(l *LabLayout, _ []Device) error {
		name, err := folderName(name)
		if err != nil {
			return err
		}
		parent := ""
		if id != "" {
			f, err := l.folder(id)
			if err != nil {
				return err
			}
			parent = f.ParentID
		} else if len(parentIDs) > 0 {
			parent = parentIDs[0]
		}
		if parent != "" {
			if _, err := l.folder(parent); err != nil {
				return err
			}
		}
		for _, f := range l.Folders {
			if f.ID != id && f.ParentID == parent && strings.EqualFold(f.Name, name) {
				return errors.New("a folder with that name already exists here")
			}
		}
		if id == "" {
			saved = Folder{ID: randomID(), Name: name, ParentID: parent}
			l.Folders = append(l.Folders, saved)
			l.setItems(parent, append(l.storedItems(parent), "folder:"+saved.ID))
			l.expandAncestors(parent)
		} else {
			folder, _ := l.folder(id)
			folder.Name = name
			saved = *folder
		}
		return nil
	})
	return saved, err
}

func (s Store) CollapseFolder(id string, collapsed bool) error {
	return s.mutateLayout(func(l *LabLayout, _ []Device) error {
		f, err := l.folder(id)
		if err != nil {
			return err
		}
		f.Collapsed = collapsed
		if !collapsed {
			l.expandAncestors(f.ParentID)
		}
		return nil
	})
}

func (s Store) DeleteFolder(id string) error {
	return s.mutateLayout(func(l *LabLayout, devices []Device) error {
		folder, err := l.folder(id)
		if err != nil {
			return err
		}
		parent := folder.ParentID
		items := l.storedItems(parent)
		index := slices.Index(items, "folder:"+id)
		children := l.storedItems(id)
		promoted := []string{}
		for _, entry := range children {
			kind, child, _ := strings.Cut(entry, ":")
			if kind == "folder" {
				f, _ := l.folder(child)
				f.ParentID = parent
			} else {
				promoted = append(promoted, child)
				if parent == "" {
					delete(l.DeviceFolders, child)
				} else {
					l.DeviceFolders[child] = parent
				}
			}
		}
		l.setItems(parent, slices.Replace(items, index, index+1, children...))
		delete(l.FolderOrder, id)
		l.Folders = slices.DeleteFunc(l.Folders, func(f Folder) bool { return f.ID == id })
		l.normalize(devices, promoted...)
		return nil
	})
}

func (l *LabLayout) placeDevice(id, parent string, index int) {
	old := l.DeviceFolders[id]
	entry := "device:" + id
	l.setItems(old, slices.DeleteFunc(l.storedItems(old), func(item string) bool { return item == entry }))
	if parent == "" {
		delete(l.DeviceFolders, id)
	} else {
		l.DeviceFolders[id] = parent
	}
	items := l.storedItems(parent)
	l.setItems(parent, slices.Insert(items, max(0, min(index, len(items))), entry))
}

func (s Store) PlaceDevice(id, parent string) error {
	return s.mutateLayout(func(l *LabLayout, devices []Device) error {
		device, err := Find(devices, id)
		if err != nil {
			return err
		}
		if parent != "" {
			if _, err := l.folder(parent); err != nil {
				return err
			}
		}
		if l.DeviceFolders[device.ID] != parent {
			index := 0
			for _, item := range l.storedItems(parent) {
				if strings.HasPrefix(item, "folder:") {
					break
				}
				index++
			}
			l.placeDevice(device.ID, parent, index)
		}
		l.expandAncestors(parent)
		return nil
	})
}

func (s Store) MoveDevice(id string, direction int) error {
	if direction != 1 && direction != -1 {
		return errors.New("move direction must be up or down")
	}
	return s.mutateLayout(func(l *LabLayout, devices []Device) error {
		device, err := Find(devices, id)
		if err != nil {
			return err
		}
		id = device.ID
		parent := l.DeviceFolders[id]
		items := l.storedItems(parent)
		index := slices.Index(items, "device:"+id)
		next := index + direction
		var adoptIDs []string
		if next >= 0 && next < len(items) {
			kind, target, _ := strings.Cut(items[next], ":")
			folder, _ := l.folder(target)
			if kind == "folder" {
				adoptIDs = []string{id}
			}
			if kind == "device" || folder.Collapsed {
				items[index], items[next] = items[next], items[index]
				l.setItems(parent, items)
			} else {
				position := 0
				if direction < 0 {
					position = len(l.storedItems(target))
				}
				l.placeDevice(id, target, position)
			}
		} else if direction < 0 && parent != "" {
			folder, _ := l.folder(parent)
			outer := l.storedItems(folder.ParentID)
			l.placeDevice(id, folder.ParentID, slices.Index(outer, "folder:"+parent))
			adoptIDs = []string{id}
		} else {
			// The next visible item can be a sibling of any ancestor.
			moved := false
			for current := parent; current != ""; {
				folder, _ := l.folder(current)
				outer := l.storedItems(folder.ParentID)
				position := slices.Index(outer, "folder:"+current)
				if direction > 0 && position+1 < len(outer) {
					kind, target, _ := strings.Cut(outer[position+1], ":")
					nextFolder, _ := l.folder(target)
					if kind == "folder" && !nextFolder.Collapsed {
						l.placeDevice(id, target, 0)
					} else {
						l.placeDevice(id, folder.ParentID, position+2)
					}
					moved = true
					break
				}
				current = folder.ParentID
			}
			if !moved {
				return errors.New("device is already at the edge of the Lab list")
			}
		}
		l.normalize(devices, adoptIDs...)
		l.expandAncestors(l.DeviceFolders[id])
		return nil
	})
}

func (s Store) MoveFolder(id string, direction int) error {
	if direction != 1 && direction != -1 {
		return errors.New("move direction must be up or down")
	}
	return s.mutateLayout(func(l *LabLayout, devices []Device) error {
		folder, err := l.folder(id)
		if err != nil {
			return err
		}
		parent := folder.ParentID
		items := l.storedItems(parent)
		index := slices.Index(items, "folder:"+id)
		next := index + direction
		if next >= 0 && next < len(items) {
			items[index], items[next] = items[next], items[index]
			l.setItems(parent, items)
		} else {
			if parent == "" {
				return errors.New("folder is already at the edge of the Lab list")
			}
			outerFolder, _ := l.folder(parent)
			outer := l.storedItems(outerFolder.ParentID)
			position := slices.Index(outer, "folder:"+parent)
			if direction > 0 {
				position++
			}
			l.setItems(parent, slices.Delete(items, index, index+1))
			folder.ParentID = outerFolder.ParentID
			l.setItems(folder.ParentID, slices.Insert(outer, position, "folder:"+id))
		}
		if !folder.Collapsed {
			items = l.storedItems(folder.ParentID)
			following := []string{}
			for _, entry := range items[slices.Index(items, "folder:"+id)+1:] {
				kind, device, _ := strings.Cut(entry, ":")
				if kind == "folder" {
					break
				}
				following = append(following, device)
			}
			l.normalize(devices, following...)
		}
		return nil
	})
}
