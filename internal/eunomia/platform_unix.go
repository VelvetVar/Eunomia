//go:build !windows

package eunomia

import (
	"os"
	"os/exec"
	"syscall"
)

func replaceFile(from, to string) error { return os.Rename(from, to) }
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
func quietCommand(cmd *exec.Cmd) {}
