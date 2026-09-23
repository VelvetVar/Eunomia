package eunomia

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func usableFile(file string) bool {
	info, err := os.Stat(file)
	return err == nil && !info.IsDir() && (runtime.GOOS == "windows" || info.Mode()&0111 != 0)
}
func Executable(tool string) (string, error) {
	variable := "EUNOMIA_" + strings.ToUpper(strings.ReplaceAll(tool, "-", "_"))
	if override := os.Getenv(variable); override != "" {
		override = strings.Trim(override, "\"")
		if filepath.IsAbs(override) && usableFile(override) {
			return override, nil
		}
		return "", fmt.Errorf("%s must point to an executable absolute path", variable)
	}
	candidates := []string{}
	if tool == "ssh-keygen" {
		if ssh, err := Executable("ssh"); err == nil {
			name := "ssh-keygen"
			if runtime.GOOS == "windows" {
				name += ".exe"
			}
			candidates = append(candidates, filepath.Join(filepath.Dir(ssh), name))
		}
	}
	lookup := tool
	if runtime.GOOS == "windows" {
		lookup += ".exe"
	}
	if file, err := exec.LookPath(lookup); err == nil {
		candidates = append(candidates, file)
	}
	if runtime.GOOS == "windows" {
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		for _, sys := range []string{"System32", "Sysnative"} {
			dir := filepath.Join(root, sys)
			if strings.HasPrefix(tool, "ssh") {
				dir = filepath.Join(dir, "OpenSSH")
			}
			candidates = append(candidates, filepath.Join(dir, tool+".exe"))
		}
		for _, base := range []string{os.Getenv("ProgramW6432"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs")} {
			if base != "" {
				for _, dir := range []string{filepath.Join("Git", "usr", "bin"), "OpenSSH"} {
					candidates = append(candidates, filepath.Join(base, dir, tool+".exe"))
				}
			}
		}
	} else {
		for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin", "/usr/local/bin", "/opt/homebrew/bin", "/run/current-system/sw/bin", "/nix/var/nix/profiles/default/bin"} {
			candidates = append(candidates, filepath.Join(dir, tool))
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates, filepath.Join(home, ".nix-profile", "bin", tool))
		}
	}
	for _, file := range candidates {
		if filepath.IsAbs(file) && usableFile(file) {
			return file, nil
		}
	}
	return "", fmt.Errorf("%s was not found; run eunomia doctor or set %s to its full path", tool, variable)
}
func SSHArgs(d Device) []string { return []string{"-p", fmt.Sprint(d.Port), "-l", d.Username, d.Host} }

type Check struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}
type Report struct {
	Platform string  `json:"platform"`
	Arch     string  `json:"arch"`
	Ready    bool    `json:"ready"`
	Checks   []Check `json:"checks"`
}

func SetupHelp(tool string) string {
	if runtime.GOOS == "windows" {
		if tool == "ssh" || tool == "ssh-keygen" {
			return "Enable the OpenSSH Client optional Windows feature, or rerun setup.cmd."
		}
		return "Windows includes ping.exe in System32; check its permissions."
	}
	if runtime.GOOS == "darwin" {
		return "macOS normally includes SSH and ping; restore the system tool or set EUNOMIA_" + strings.ToUpper(strings.ReplaceAll(tool, "-", "_"))
	}
	if strings.HasPrefix(tool, "ssh") {
		return "Install openssh-client (Debian/Ubuntu), openssh-clients (Fedora), or rerun setup.sh."
	}
	return "Install iputils-ping (Debian/Ubuntu) or iputils (Fedora/Arch)."
}
func Doctor() Report {
	r := Report{runtime.GOOS, runtime.GOARCH, true, []Check{{ID: "runtime", Label: "Native Go executable", Status: "ok", Detail: runtime.Version() + "; no Node.js runtime required"}}}
	for _, tool := range []string{"ssh", "ssh-keygen", "ping"} {
		file, err := Executable(tool)
		if err == nil && tool == "ssh" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			cmd := exec.CommandContext(ctx, file, "-V")
			quietCommand(cmd)
			var bytes []byte
			bytes, err = cmd.CombinedOutput()
			cancel()
			if err == nil && !strings.Contains(string(bytes), "OpenSSH") {
				err = fmt.Errorf("the selected SSH client did not identify itself as OpenSSH")
			}
			file += " (" + strings.TrimSpace(safe(string(bytes))) + ")"
		}
		c := Check{ID: tool, Label: tool, Status: "ok", Detail: file}
		if err != nil {
			c.Status = "warning"
			if tool == "ssh" {
				c.Status = "error"
				r.Ready = false
			}
			c.Detail = err.Error()
			c.Fix = SetupHelp(tool)
		}
		r.Checks = append(r.Checks, c)
	}
	if err := terminalSupport(); err != nil {
		r.Ready = false
		r.Checks = append(r.Checks, Check{ID: "terminal", Label: "Terminal support", Status: "error", Detail: err.Error()})
	} else {
		r.Checks = append(r.Checks, Check{ID: "terminal", Label: "Terminal support", Status: "ok", Detail: "Native PTY support available"})
	}
	return r
}
