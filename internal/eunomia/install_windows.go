//go:build windows

package eunomia

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"strings"
	"unsafe"
)

func registerPath(bin string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	current, kind, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	for _, entry := range strings.Split(current, ";") {
		if strings.EqualFold(strings.TrimRight(entry, `\`), strings.TrimRight(bin, `\`)) {
			return nil
		}
	}
	updated := bin + ";" + current
	if kind == registry.EXPAND_SZ {
		err = key.SetExpandStringValue("Path", updated)
	} else {
		err = key.SetStringValue("Path", updated)
	}
	if err != nil {
		return err
	}
	message, _ := windows.UTF16PtrFromString("Environment")
	send := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	var result uintptr
	send.Call(0xffff, 0x1a, 0, uintptr(unsafe.Pointer(message)), 2, 1000, uintptr(unsafe.Pointer(&result)))
	return nil
}
