package eunomia

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type Device struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Username    string `json:"username"`
	Port        int    `json:"port"`
	Description string `json:"description"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}
type deviceFile struct {
	Version int        `json:"version"`
	Devices []Device   `json:"devices"`
	Layout  *LabLayout `json:"lab,omitempty"`
}
type Store struct{ Directory string }

func ConfigDir() string {
	if value := os.Getenv("EUNOMIA_HOME"); value != "" {
		absolute, _ := filepath.Abs(value)
		return absolute
	}
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("APPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, "Eunomia")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Eunomia")
	default:
		base := os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = filepath.Join(home, ".config")
		}
		return filepath.Join(base, "eunomia")
	}
}
func (s Store) Path() string { return filepath.Join(s.Directory, "devices.json") }
func safe(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
}

var hostLabel = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
var numericHost = regexp.MustCompile(`^[0-9.]+$`)
var validUser = regexp.MustCompile(`^[a-zA-Z0-9_][a-zA-Z0-9_.@\\$-]*$`)

func Validate(d Device) (Device, error) {
	fields := []struct {
		value    *string
		name     string
		limit    int
		required bool
	}{{&d.Name, "Name", 60, true}, {&d.Host, "Address", 253, true}, {&d.Username, "Default user", 64, true}, {&d.Description, "Description", 240, false}}
	for _, f := range fields {
		*f.value = strings.TrimSpace(*f.value)
		if f.required && *f.value == "" {
			return d, fmt.Errorf("%s is required", f.name)
		}
		if !utf8.ValidString(*f.value) || utf8.RuneCountInString(*f.value) > f.limit || safe(*f.value) != *f.value {
			return d, fmt.Errorf("%s must be at most %d characters without control characters", f.name, f.limit)
		}
	}
	if net.ParseIP(d.Host) == nil {
		if numericHost.MatchString(d.Host) {
			return d, errors.New("enter a valid IP address or hostname")
		}
		for _, label := range strings.Split(strings.TrimSuffix(d.Host, "."), ".") {
			if !hostLabel.MatchString(label) {
				return d, errors.New("enter a valid IP address or hostname without a URL scheme")
			}
		}
	}
	if !validUser.MatchString(d.Username) {
		return d, errors.New("default user contains unsupported characters")
	}
	if d.Port < 1 || d.Port > 65535 {
		return d, errors.New("SSH port must be between 1 and 65535")
	}
	return d, nil
}
func (s Store) Read() ([]Device, error) {
	data, err := s.readData()
	return data.Devices, err
}

func decodeDeviceFile(bytes []byte) (deviceFile, error) {
	var data deviceFile
	err := json.Unmarshal(bytes, &data)
	if err != nil || (data.Version != 1 && data.Version != 2) || data.Devices == nil || (data.Version == 2 && data.Layout == nil) {
		return data, errors.New("invalid profile format; original file left untouched")
	}
	ids, names := map[string]bool{}, map[string]bool{}
	for i, d := range data.Devices {
		clean, err := Validate(d)
		if err != nil {
			return data, fmt.Errorf("invalid stored device: %w; original file left untouched", err)
		}
		if d.ID == "" || safe(d.ID) != d.ID || ids[d.ID] || names[strings.ToLower(clean.Name)] {
			return data, errors.New("invalid or duplicate stored device; original file left untouched")
		}
		if _, err = time.Parse(time.RFC3339Nano, d.CreatedAt); err != nil {
			return data, errors.New("invalid device creation date")
		}
		if _, err = time.Parse(time.RFC3339Nano, d.UpdatedAt); err != nil {
			return data, errors.New("invalid device update date")
		}
		ids[d.ID] = true
		names[strings.ToLower(clean.Name)] = true
		data.Devices[i] = clean
	}
	return data, nil
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}
func writeAtomic(file string, bytes []byte, mode os.FileMode) error {
	temp, err := os.CreateTemp(filepath.Dir(file), ".eunomia-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(mode); err == nil {
		_, err = temp.Write(bytes)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceFile(name, file)
}

// writeNewFile never overwrites an existing file and removes its own incomplete
// output if writing, flushing, or closing fails.
func writeNewFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(path)
	}
	return err
}

func Find(devices []Device, ref string) (Device, error) {
	for _, d := range devices {
		if d.ID == ref || strings.EqualFold(d.Name, ref) {
			return d, nil
		}
	}
	return Device{}, fmt.Errorf("device not found: %s", safe(ref))
}
func Search(devices []Device, query string) []Device {
	result := []Device{}
	query = strings.ToLower(query)
	for _, d := range devices {
		if strings.Contains(strings.ToLower(strings.Join([]string{d.Name, d.Host, d.Username, d.Description}, "\n")), query) {
			result = append(result, d)
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name) })
	return result
}
func (s Store) Save(d Device, ref string) (Device, error) {
	return s.save(d, ref, "")
}

// SaveInFolder creates the profile and its membership in the same transaction.
// If the destination was removed while the form was open, nothing is saved.
func (s Store) SaveInFolder(d Device, folder string) (Device, error) {
	return s.save(d, "", folder)
}

func (s Store) save(d Device, ref, folder string) (Device, error) {
	var saved Device
	err := s.mutateData(func(data *deviceFile) error {
		devices := data.Devices
		clean, err := Validate(d)
		if err != nil {
			return err
		}
		if folder != "" {
			if _, err := data.Layout.folder(folder); err != nil {
				return err
			}
		}
		index := -1
		if ref != "" {
			old, err := Find(devices, ref)
			if err != nil {
				return err
			}
			clean.ID = old.ID
			clean.CreatedAt = old.CreatedAt
			for i := range devices {
				if devices[i].ID == old.ID {
					index = i
				}
			}
		} else {
			clean.ID = randomID()
			clean.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		for i, other := range devices {
			if i != index && strings.EqualFold(other.Name, clean.Name) {
				return errors.New("a device with that name already exists")
			}
		}
		clean.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		saved = clean
		if index < 0 {
			devices = append(devices, clean)
		} else {
			devices[index] = clean
		}
		data.Devices = devices
		if folder != "" {
			data.Layout.placeInFolder(clean.ID, folder)
		}
		return nil
	})
	return saved, err
}
func (s Store) Remove(ref string) error {
	return s.mutateData(func(data *deviceFile) error {
		devices := data.Devices
		d, err := Find(devices, ref)
		if err != nil {
			return err
		}
		for i := range devices {
			if devices[i].ID == d.ID {
				data.Devices = append(devices[:i], devices[i+1:]...)
				return nil
			}
		}
		return nil
	})
}
