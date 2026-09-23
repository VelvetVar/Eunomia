package eunomia

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func Install(root, bin string, noPath bool, out io.Writer) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if root == "" {
		root = os.Getenv("EUNOMIA_INSTALL_ROOT")
	}
	if root == "" {
		if runtime.GOOS == "windows" {
			root = filepath.Join(os.Getenv("LOCALAPPDATA"), "Eunomia")
		} else {
			root = filepath.Join(home, ".local", "share", "eunomia")
		}
	}
	if bin == "" {
		bin = os.Getenv("EUNOMIA_BIN_DIR")
	}
	if bin == "" {
		if runtime.GOOS == "windows" {
			bin = filepath.Join(root, "bin")
		} else {
			bin = filepath.Join(home, ".local", "bin")
		}
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(bin) || strings.ContainsAny(root+bin, "\r\n") {
		return errors.New("installation and command directories must be absolute paths without newlines")
	}
	report := Doctor()
	printReport(out, report)
	if !report.Ready {
		return errors.New("resolve the startup errors above before installing; the previous command is unchanged")
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	bytes, err := os.ReadFile(executable)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(bytes)
	digest := hex.EncodeToString(sum[:])
	name := "eunomia"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	release := filepath.Join(root, "releases", Version+"-"+digest[:12])
	if err = os.MkdirAll(release, 0700); err != nil {
		return err
	}
	if err = os.MkdirAll(bin, 0755); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(root, "install.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("installation is locked; ensure no installer is running before removing install.lock: %w", err)
	}
	defer func() { lock.Close(); os.Remove(lock.Name()) }()
	releaseFile := filepath.Join(release, name)
	if existing, readErr := os.ReadFile(releaseFile); readErr != nil || sha256.Sum256(existing) != sum {
		if err = writeAtomic(releaseFile, bytes, 0755); err != nil {
			return err
		}
	}
	target := filepath.Join(bin, name)
	if existing, readErr := os.ReadFile(target); readErr != nil || sha256.Sum256(existing) != sum {
		if err = writeAtomic(target, bytes, 0755); err != nil {
			return fmt.Errorf("cannot replace %s: run eunomia down and retry; previous executable preserved: %w", target, err)
		}
	}
	if !noPath {
		if err = registerPath(bin); err != nil {
			return fmt.Errorf("executable installed at %s, but PATH registration failed: %w", target, err)
		}
	}
	fmt.Fprintf(out, "\nInstalled Eunomia %s (Go) to %s\nNo Node.js, npm, Go toolchain, or background service is required.\nOpen a new terminal and run: eunomia up\nFrom another terminal: eunomia down\nSaved devices remain at %s\n", Version, target, (Store{ConfigDir()}).Path())
	return nil
}
