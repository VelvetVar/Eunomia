//go:build !windows

package eunomia

// Other platforms continue to use their terminal's paste shortcut.
var systemClipboard func() (string, error)
