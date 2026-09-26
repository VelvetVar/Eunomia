//go:build windows

package eunomia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"time"

	"golang.org/x/sys/windows"
)

var systemClipboard = readWindowsClipboard

var (
	clipboardUser32          = windows.NewLazySystemDLL("user32.dll")
	clipboardKernel32        = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard            = clipboardUser32.NewProc("OpenClipboard")
	closeClipboard           = clipboardUser32.NewProc("CloseClipboard")
	clipboardFormatAvailable = clipboardUser32.NewProc("IsClipboardFormatAvailable")
	getClipboardData         = clipboardUser32.NewProc("GetClipboardData")
	clipboardGlobalSize      = clipboardKernel32.NewProc("GlobalSize")
	clipboardGlobalLock      = clipboardKernel32.NewProc("GlobalLock")
	clipboardGlobalUnlock    = clipboardKernel32.NewProc("GlobalUnlock")
)

func readWindowsClipboard() (string, error) {
	// tcell's Windows GetClipboard is a no-op. Read Unicode text directly,
	// keeping the open/copy/close sequence on one Windows thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for attempt := 0; ; attempt++ {
		if ok, _, _ := openClipboard.Call(0); ok != 0 {
			break
		}
		if attempt == 4 {
			return "", errors.New("clipboard is busy; right-click again")
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer closeClipboard.Call()
	const unicodeText = 13 // CF_UNICODETEXT
	if available, _, _ := clipboardFormatAvailable.Call(unicodeText); available == 0 {
		return "", nil
	}
	handle, _, err := getClipboardData.Call(unicodeText)
	if handle == 0 {
		return "", fmt.Errorf("cannot read clipboard: %w", err)
	}
	size, _, _ := clipboardGlobalSize.Call(handle)
	if size < 2 || size%2 != 0 {
		return "", errors.New("clipboard text has an invalid size")
	}
	if size > 1<<20 {
		return "", errors.New("clipboard text is too large (maximum 1 MiB)")
	}
	address, _, err := clipboardGlobalLock.Call(handle)
	if address == 0 {
		return "", fmt.Errorf("cannot access clipboard text: %w", err)
	}
	defer clipboardGlobalUnlock.Call(handle)
	data := make([]byte, int(size))
	// Copy from this process's locked clipboard allocation before releasing it.
	// ReadProcessMemory bounds the copy without converting an integer address
	// into a Go pointer or assuming that the clipboard text is NUL-terminated.
	if err := windows.ReadProcessMemory(windows.CurrentProcess(), address, &data[0], size, nil); err != nil {
		return "", fmt.Errorf("cannot copy clipboard text: %w", err)
	}
	return decodeClipboardText(data)
}

func decodeClipboardText(data []byte) (string, error) {
	if len(data)%2 != 0 {
		return "", errors.New("clipboard text has an invalid size")
	}
	text := make([]uint16, 0, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		unit := binary.LittleEndian.Uint16(data[i:])
		if unit == 0 {
			return windows.UTF16ToString(text), nil
		}
		text = append(text, unit)
	}
	return "", errors.New("clipboard text is missing its terminator")
}
