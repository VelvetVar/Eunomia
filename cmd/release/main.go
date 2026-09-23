// Build self-contained native executables and release archives. Run: go run ./cmd/release
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type entry struct {
	Name       string
	Data       []byte
	Executable bool
}

var epoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func checksum(bytes []byte) string { sum := sha256.Sum256(bytes); return hex.EncodeToString(sum[:]) }
func archive(entries []entry, windows bool) ([]byte, error) {
	var output bytes.Buffer
	if windows {
		writer := zip.NewWriter(&output)
		for _, file := range entries {
			header := &zip.FileHeader{Name: file.Name, Method: zip.Deflate}
			header.SetModTime(epoch)
			header.SetMode(0644)
			if file.Executable {
				header.SetMode(0755)
			}
			w, err := writer.CreateHeader(header)
			if err != nil {
				return nil, err
			}
			if _, err = w.Write(file.Data); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
	} else {
		gz := gzip.NewWriter(&output)
		writer := tar.NewWriter(gz)
		for _, file := range entries {
			mode := int64(0644)
			if file.Executable {
				mode = 0755
			}
			if err := writer.WriteHeader(&tar.Header{Name: file.Name, Mode: mode, Size: int64(len(file.Data)), ModTime: epoch, Typeflag: tar.TypeReg}); err != nil {
				return nil, err
			}
			if _, err := writer.Write(file.Data); err != nil {
				return nil, err
			}
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
		if err := gz.Close(); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}
func buildEnv(target, arch string) []string {
	result := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if key != "GOOS" && key != "GOARCH" && key != "CGO_ENABLED" {
			result = append(result, item)
		}
	}
	return append(result, "GOOS="+target, "GOARCH="+arch, "CGO_ENABLED=0")
}
func run() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if _, err = os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return fmt.Errorf("run this command from the Eunomia source folder")
	}
	out := filepath.Join(root, "dist")
	if len(os.Args) > 1 {
		out, err = filepath.Abs(os.Args[1])
		if err != nil {
			return err
		}
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		return err
	}
	goTool := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goTool += ".exe"
	}
	var wg sync.WaitGroup
	slots := make(chan struct{}, 2)
	failures := make(chan error, 6)
	for _, target := range []string{"windows", "linux", "darwin"} {
		for _, arch := range []string{"amd64", "arm64"} {
			wg.Add(1)
			go func(target, arch string) {
				defer wg.Done()
				slots <- struct{}{}
				defer func() { <-slots }()
				name := "eunomia"
				if target == "windows" {
					name += ".exe"
				}
				directory := filepath.Join(out, "bin", target+"-"+arch)
				if err := os.MkdirAll(directory, 0755); err != nil {
					failures <- err
					return
				}
				file := filepath.Join(directory, name)
				command := exec.Command(goTool, "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w", "-o", file, "./cmd/eunomia")
				command.Env = buildEnv(target, arch)
				command.Dir = root
				if output, err := command.CombinedOutput(); err != nil {
					failures <- fmt.Errorf("%s/%s: %w\n%s", target, arch, err, output)
					return
				}
				bytes, err := os.ReadFile(file)
				if err != nil {
					failures <- err
					return
				}
				if err = os.WriteFile(file+".sha256", []byte(checksum(bytes)+"  "+name+"\n"), 0644); err != nil {
					failures <- err
					return
				}
				fmt.Printf("Built %s/%s: %.1f MiB\n", target, arch, float64(len(bytes))/(1024*1024))
			}(target, arch)
		}
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			return err
		}
	}
	licenseFiles, err := dependencyLicenses(goTool)
	if err != nil {
		return err
	}
	var sums strings.Builder
	for _, target := range []string{"windows", "linux"} {
		prefix := "eunomia-" + target
		files := []string{"README.md", "CONTRIBUTING.md", "docs/installation.md", "docs/usage.md", "docs/troubleshooting.md", "LICENSE", "setup.sh"}
		if target == "windows" {
			files = append(files, "setup.ps1", "setup.cmd")
		}
		entries := []entry{}
		for _, name := range files {
			bytes, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				return err
			}
			if strings.HasSuffix(name, ".sh") {
				bytes = []byte(strings.ReplaceAll(string(bytes), "\r\n", "\n"))
			}
			entries = append(entries, entry{prefix + "/" + name, bytes, strings.HasSuffix(name, ".sh")})
		}
		for _, file := range licenseFiles {
			entries = append(entries, entry{prefix + "/licenses/" + file.Name, file.Data, false})
		}
		for _, arch := range []string{"amd64", "arm64"} {
			name := "eunomia"
			if target == "windows" {
				name += ".exe"
			}
			for _, suffix := range []string{"", ".sha256"} {
				relative := filepath.ToSlash(filepath.Join("bin", target+"-"+arch, name+suffix))
				bytes, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(relative)))
				if err != nil {
					return err
				}
				entries = append(entries, entry{prefix + "/" + relative, bytes, suffix == ""})
			}
		}
		instruction := "Extract this package and run: sh ./setup.sh\n"
		if target == "windows" {
			instruction = "Extract this ZIP and run .\\setup.cmd in PowerShell or CMD.\n"
		}
		instruction += "Open a new terminal: eunomia up\nFrom a second terminal: eunomia down\nD = Discover; Delete = remove a saved device.\n\nThis Go release includes x64 and ARM64 executables. No Node.js, npm or Go installation is required. Setup can run offline when OpenSSH is installed. Missing OS SSH/ping tools may require administrator access and a network connection. Existing devices.json profiles are reused. See README.md for options and migration notes.\n"
		entries = append(entries, entry{prefix + "/INSTALL.txt", []byte(instruction), false})
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
		var manifest strings.Builder
		for _, file := range entries {
			fmt.Fprintf(&manifest, "%s  %s\n", checksum(file.Data), strings.TrimPrefix(file.Name, prefix+"/"))
		}
		entries = append(entries, entry{prefix + "/MANIFEST.sha256", []byte(manifest.String()), false})
		bytes, err := archive(entries, target == "windows")
		if err != nil {
			return err
		}
		extension := ".tar.gz"
		if target == "windows" {
			extension = ".zip"
		}
		name := prefix + extension
		if err = os.WriteFile(filepath.Join(out, name), bytes, 0644); err != nil {
			return err
		}
		fmt.Fprintf(&sums, "%s  %s\n", checksum(bytes), name)
		fmt.Printf("Packaged %s (%.1f MiB)\n", name, float64(len(bytes))/(1024*1024))
	}
	return os.WriteFile(filepath.Join(out, "SHA256SUMS.txt"), []byte(sums.String()), 0644)
}
func dependencyLicenses(goTool string) ([]entry, error) {
	cmd := exec.Command(goTool, "list", "-m", "-f", "{{if .Dir}}{{.Path}}|{{.Dir}}{{end}}", "all")
	bytes, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	files := []entry{}
	for _, line := range strings.Split(strings.TrimSpace(string(bytes)), "\n") {
		module, dir, ok := strings.Cut(line, "|")
		if !ok || module == "eunomia" {
			continue
		}
		entries, err := os.ReadDir(strings.TrimSpace(dir))
		if err != nil {
			return nil, err
		}
		for _, item := range entries {
			name := strings.ToUpper(item.Name())
			if !item.IsDir() && (strings.HasPrefix(name, "LICENSE") || strings.HasPrefix(name, "COPYING") || strings.HasPrefix(name, "NOTICE")) {
				data, err := os.ReadFile(filepath.Join(strings.TrimSpace(dir), item.Name()))
				if err != nil {
					return nil, err
				}
				files = append(files, entry{strings.ReplaceAll(module, "/", "_") + "_" + item.Name(), data, false})
			}
		}
	}
	return files, nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
