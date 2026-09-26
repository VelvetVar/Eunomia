//go:build !windows

package eunomia

// Other platforms continue to use their terminal's paste shortcut.
var systemClipboard func() (string, error)

// Use the terminal's clipboard protocol for copying on other platforms.
var systemClipboardWriter func(string) error
