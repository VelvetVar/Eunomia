package eunomia

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// devices.json is the sole active profile: devices and the complete Lab tree
// share a writer lock and an atomic replacement.
func (s Store) LayoutPath() string       { return s.Path() }
func (s Store) legacyLayoutPath() string { return filepath.Join(s.Directory, "lab.json") }

func (s Store) readData() (deviceFile, error) {
	raw, err := os.ReadFile(s.Path())
	data := deviceFile{Version: 1, Devices: []Device{}}
	if err == nil {
		data, err = decodeDeviceFile(raw)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return data, err
	}
	if data.Layout == nil {
		layout := newLabLayout()
		if raw, err := os.ReadFile(s.legacyLayoutPath()); err == nil {
			layout = LabLayout{}
			if err := json.Unmarshal(raw, &layout); err != nil {
				return data, fmt.Errorf("invalid legacy Lab layout: %w", err)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return data, err
		}
		data.Layout = &layout
	}
	if err := data.Layout.validate(); err != nil {
		return data, err
	}
	data.Layout.normalize(data.Devices)
	return data, nil
}

func (s Store) ReadAll() ([]Device, LabLayout, error) {
	data, err := s.readData()
	if err != nil {
		return nil, LabLayout{}, err
	}
	return data.Devices, *data.Layout, nil
}

func (s Store) ReadLayout() (LabLayout, error) {
	_, layout, err := s.ReadAll()
	return layout, err
}

func (s Store) Migrate() error {
	data, err := s.readData()
	if err != nil || data.Version == 2 {
		return err
	}
	if _, err := os.Stat(s.Path()); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(s.legacyLayoutPath()); errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	return s.mutateData(func(*deviceFile) error { return nil })
}

func (s Store) mutateData(change func(*deviceFile) error) error {
	if err := os.MkdirAll(s.Directory, 0700); err != nil {
		return err
	}
	lock := s.Path() + ".lock"
	handle, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("profile is locked or inaccessible: %w", err)
	}
	defer func() { handle.Close(); os.Remove(lock) }()
	data, err := s.readData()
	if err != nil {
		return err
	}
	legacy := data.Version == 1
	if err := change(&data); err != nil {
		return err
	}
	if err := data.Layout.validate(); err != nil {
		return err
	}
	data.Layout.normalize(data.Devices)
	data.Version = 2
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	if legacy {
		if original, err := os.ReadFile(s.Path()); err == nil {
			backup, err := os.OpenFile(s.Path()+".v1-backup", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err == nil {
				_, writeErr := backup.Write(original)
				closeErr := backup.Close()
				if writeErr != nil {
					os.Remove(s.Path() + ".v1-backup")
					return writeErr
				}
				if closeErr != nil {
					os.Remove(s.Path() + ".v1-backup")
					return closeErr
				}
			} else if !errors.Is(err, os.ErrExist) {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return writeAtomic(s.Path(), append(raw, '\n'), 0600)
}

func (s Store) Export(path string) error {
	data, err := s.readData()
	if err != nil {
		return err
	}
	data.Version = 2
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("choose a new export filename: %w", err)
	}
	_, writeErr := file.Write(append(raw, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		os.Remove(path)
		return writeErr
	}
	return closeErr
}

func (s Store) Import(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	data, err := decodeDeviceFile(raw)
	if err != nil {
		return err
	}
	if data.Layout == nil {
		layout := newLabLayout()
		data.Layout = &layout
	}
	if err := data.Layout.validate(); err != nil {
		return err
	}
	known := map[string]bool{}
	for _, device := range data.Devices {
		known[device.ID] = true
	}
	for id := range data.Layout.DeviceFolders {
		if !known[id] {
			return errors.New("import refers to an unknown device")
		}
	}
	for _, id := range data.Layout.Order {
		if !known[id] {
			return errors.New("import order refers to an unknown device")
		}
	}
	orders := map[string][]string{"": data.Layout.RootOrder}
	for id, items := range data.Layout.FolderOrder {
		orders[id] = items
	}
	for parent, items := range orders {
		for _, entry := range items {
			kind, id, _ := strings.Cut(entry, ":")
			if kind == "folder" {
				folder, err := data.Layout.folder(id)
				if err != nil || folder.ParentID != parent {
					return errors.New("import has an invalid folder position")
				}
			} else if !known[id] || data.Layout.DeviceFolders[id] != parent {
				return errors.New("import has an invalid device position")
			}
		}
	}
	data.Layout.normalize(data.Devices)
	return s.mutateData(func(current *deviceFile) error { *current = data; return nil })
}
