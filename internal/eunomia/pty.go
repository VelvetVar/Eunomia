package eunomia

import (
	"io"
	"os"
	"strings"
)

func terminalEnv() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, "TERM") {
			env = append(env, entry)
		}
	}
	return append(env, "TERM=xterm-256color")
}

type TerminalProcess interface {
	io.ReadWriteCloser
	Resize(width, height int) error
	Wait() (int, error)
}
