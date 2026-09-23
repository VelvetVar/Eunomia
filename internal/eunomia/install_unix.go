//go:build !windows

package eunomia

import (
	"os"
	"path/filepath"
	"strings"
)

func shellQuote(text string) string { return "'" + strings.ReplaceAll(text, "'", "'\\''") + "'" }
func registerPath(bin string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	stanza := "\n# Eunomia native command\nexport PATH=" + shellQuote(bin) + ":\"$PATH\"\n"
	files := []string{".profile", ".bashrc"}
	// A login bash reads only the first existing login profile.
	for _, name := range []string{".bash_profile", ".bash_login"} {
		if _, err := os.Stat(filepath.Join(home, name)); err == nil {
			files = append(files, name)
			break
		}
	}
	if strings.Contains(os.Getenv("SHELL"), "zsh") {
		files = append(files, ".zshrc")
	}
	for _, name := range files {
		file := filepath.Join(home, name)
		contents, err := os.ReadFile(file)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if strings.Contains(string(contents), strings.TrimSpace(stanza)) {
			continue
		}
		handle, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, err = handle.WriteString(stanza)
		handle.Close()
		if err != nil {
			return err
		}
	}
	if strings.Contains(os.Getenv("SHELL"), "fish") {
		dir := filepath.Join(home, ".config", "fish", "conf.d")
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		return writeAtomic(filepath.Join(dir, "eunomia.fish"), []byte("fish_add_path "+shellQuote(bin)+"\n"), 0600)
	}
	return nil
}
