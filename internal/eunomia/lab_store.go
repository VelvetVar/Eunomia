package eunomia

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

type Folder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Collapsed bool   `json:"collapsed,omitempty"`
}

// Organization is separate from devices.json so older clients and CLI profile
// edits cannot discard it. Missing device IDs are pruned on the next write.
type LabLayout struct {
	Version       int               `json:"version"`
	Folders       []Folder          `json:"folders"`
	Order         []string          `json:"order"`
	DeviceFolders map[string]string `json:"deviceFolders"`
}

func (s Store) LayoutPath() string { return filepath.Join(s.Directory, "lab.json") }

func folderName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > 60 || safe(name) != name {
		return "", errors.New("folder name must be 1–60 characters without control characters")
	}
	return name, nil
}

func (s Store) ReadLayout() (LabLayout, error) {
	layout := LabLayout{Version: 1, Folders: []Folder{}, Order: []string{}, DeviceFolders: map[string]string{}}
	data, err := os.ReadFile(s.LayoutPath())
	if errors.Is(err, os.ErrNotExist) {
		return layout, nil
	}
	if err != nil {
		return layout, err
	}
	var stored LabLayout
	if err := json.Unmarshal(data, &stored); err != nil || stored.Version != 1 || stored.Folders == nil || stored.Order == nil || stored.DeviceFolders == nil {
		return layout, fmt.Errorf("cannot read %s: invalid Lab layout; original file left untouched", s.LayoutPath())
	}
	ids, names := map[string]bool{}, map[string]bool{}
	validID := func(id string) bool { return id != "" && len(id) <= 256 && safe(id) == id && utf8.ValidString(id) }
	for i, folder := range stored.Folders {
		name, err := folderName(folder.Name)
		if err != nil || !validID(folder.ID) || ids[folder.ID] || names[strings.ToLower(name)] {
			return layout, errors.New("invalid or duplicate folder; original Lab layout left untouched")
		}
		ids[folder.ID], names[strings.ToLower(name)] = true, true
		stored.Folders[i].Name = name
	}
	seen := map[string]bool{}
	for _, id := range stored.Order {
		if !validID(id) || seen[id] {
			return layout, errors.New("invalid device order; original Lab layout left untouched")
		}
		seen[id] = true
	}
	for device, folder := range stored.DeviceFolders {
		if !validID(device) || !ids[folder] {
			return layout, errors.New("invalid folder assignment; original Lab layout left untouched")
		}
	}
	return stored, nil
}

func (l LabLayout) search(devices []Device, query string) []Device {
	result := Search(devices, query)
	ranks := make(map[string]int, len(l.Order))
	for i, id := range l.Order {
		ranks[id] = i
	}
	rank := func(id string) int {
		if order, ok := ranks[id]; ok {
			return order
		}
		return len(l.Order)
	}
	sort.SliceStable(result, func(i, j int) bool { return rank(result[i].ID) < rank(result[j].ID) })
	return result
}

func (s Store) mutateLayout(change func(*LabLayout, []Device) error) error {
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return err
	}
	lock := s.LayoutPath() + ".lock"
	handle, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("Lab layout is locked or inaccessible: %w", err)
	}
	defer func() { handle.Close(); os.Remove(lock) }()
	layout, err := s.ReadLayout()
	if err != nil {
		return err
	}
	devices, err := s.Read()
	if err != nil {
		return err
	}
	ordered := layout.search(devices, "")
	layout.Order = []string{}
	known := map[string]bool{}
	for _, device := range ordered {
		layout.Order = append(layout.Order, device.ID)
		known[device.ID] = true
	}
	for device := range layout.DeviceFolders {
		if !known[device] {
			delete(layout.DeviceFolders, device)
		}
	}
	if err := change(&layout, devices); err != nil {
		return err
	}
	data, err := json.MarshalIndent(layout, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.LayoutPath(), append(data, '\n'), 0600)
}

func (l *LabLayout) folder(id string) (*Folder, error) {
	for i := range l.Folders {
		if l.Folders[i].ID == id {
			return &l.Folders[i], nil
		}
	}
	return nil, errors.New("folder no longer exists; reload Lab")
}

func (s Store) SaveFolder(name, id string) (Folder, error) {
	var saved Folder
	err := s.mutateLayout(func(layout *LabLayout, _ []Device) error {
		name, err := folderName(name)
		if err != nil {
			return err
		}
		for _, folder := range layout.Folders {
			if folder.ID != id && strings.EqualFold(folder.Name, name) {
				return errors.New("a folder with that name already exists")
			}
		}
		if id == "" {
			saved = Folder{ID: randomID(), Name: name}
			layout.Folders = append(layout.Folders, saved)
			return nil
		}
		folder, err := layout.folder(id)
		if err != nil {
			return err
		}
		folder.Name = name
		saved = *folder
		return nil
	})
	return saved, err
}

func (s Store) CollapseFolder(id string, collapsed bool) error {
	return s.mutateLayout(func(layout *LabLayout, _ []Device) error {
		folder, err := layout.folder(id)
		if err != nil {
			return err
		}
		folder.Collapsed = collapsed
		return nil
	})
}

func (s Store) DeleteFolder(id string) error {
	return s.mutateLayout(func(layout *LabLayout, _ []Device) error {
		if _, err := layout.folder(id); err != nil {
			return err
		}
		layout.Folders = slices.DeleteFunc(layout.Folders, func(folder Folder) bool { return folder.ID == id })
		for device, folder := range layout.DeviceFolders {
			if folder == id {
				delete(layout.DeviceFolders, device)
			}
		}
		return nil
	})
}

func (s Store) MoveDevice(id string, direction int) error {
	if direction != -1 && direction != 1 {
		return errors.New("move direction must be up or down")
	}
	return s.mutateLayout(func(layout *LabLayout, devices []Device) error {
		device, err := Find(devices, id)
		if err != nil {
			return err
		}
		index := slices.Index(layout.Order, device.ID)
		for next := index + direction; next >= 0 && next < len(layout.Order); next += direction {
			if layout.DeviceFolders[layout.Order[next]] == layout.DeviceFolders[device.ID] {
				layout.Order[index], layout.Order[next] = layout.Order[next], layout.Order[index]
				return nil
			}
		}
		return errors.New("device is already at the edge of this group")
	})
}

func (s Store) PlaceDevice(id, folderID string) error {
	return s.mutateLayout(func(layout *LabLayout, devices []Device) error {
		device, err := Find(devices, id)
		if err != nil {
			return err
		}
		if folderID != "" {
			folder, err := layout.folder(folderID)
			if err != nil {
				return err
			}
			folder.Collapsed = false
		}
		if layout.DeviceFolders[device.ID] == folderID {
			return nil
		}
		if folderID == "" {
			delete(layout.DeviceFolders, device.ID)
		} else {
			layout.DeviceFolders[device.ID] = folderID
		}
		layout.Order = slices.DeleteFunc(layout.Order, func(id string) bool { return id == device.ID })
		layout.Order = append(layout.Order, device.ID)
		return nil
	})
}
