//go:build windows

package eunomia

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var systemClipboard = readWindowsClipboard
var systemClipboardWriter = writeWindowsClipboard

const unicodeClipboardText = 13 // CF_UNICODETEXT

var (
	clipboardUser32          = windows.NewLazySystemDLL("user32.dll")
	clipboardKernel32        = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard            = clipboardUser32.NewProc("OpenClipboard")
	closeClipboard           = clipboardUser32.NewProc("CloseClipboard")
	clipboardFormatAvailable = clipboardUser32.NewProc("IsClipboardFormatAvailable")
	getClipboardData         = clipboardUser32.NewProc("GetClipboardData")
	setClipboardData         = clipboardUser32.NewProc("SetClipboardData")
	emptyClipboard           = clipboardUser32.NewProc("EmptyClipboard")
	clipboardCreateWindow    = clipboardUser32.NewProc("CreateWindowExW")
	clipboardDestroyWindow   = clipboardUser32.NewProc("DestroyWindow")
	clipboardGlobalAlloc     = clipboardKernel32.NewProc("GlobalAlloc")
	clipboardGlobalFree      = clipboardKernel32.NewProc("GlobalFree")
	clipboardGlobalSize      = clipboardKernel32.NewProc("GlobalSize")
	clipboardGlobalLock      = clipboardKernel32.NewProc("GlobalLock")
	clipboardGlobalUnlock    = clipboardKernel32.NewProc("GlobalUnlock")
)

// The caller keeps its goroutine on one OS thread until CloseClipboard.
func lockWindowsClipboard(owner uintptr) error {
	for attempt := 0; ; attempt++ {
		if ok, _, _ := openClipboard.Call(owner); ok != 0 {
			return nil
		}
		if attempt == 4 {
			return errors.New("clipboard is busy; try again")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func readWindowsClipboard() (string, error) {
	// tcell's Windows GetClipboard is a no-op. Read Unicode text directly,
	// keeping the open/copy/close sequence on one Windows thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := lockWindowsClipboard(0); err != nil {
		return "", err
	}
	defer closeClipboard.Call()
	if available, _, _ := clipboardFormatAvailable.Call(unicodeClipboardText); available == 0 {
		return "", nil
	}
	handle, _, err := getClipboardData.Call(unicodeClipboardText)
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

func encodeClipboardText(text string) ([]byte, error) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
	units, err := windows.UTF16FromString(text)
	if err != nil {
		return nil, errors.New("selected text contains a NUL character")
	}
	if len(units)*2 > 1<<20 {
		return nil, errors.New("selected text is too large (maximum 1 MiB)")
	}
	data := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(data[i*2:], unit)
	}
	return data, nil
}

func writeWindowsClipboard(text string) error {
	data, err := encodeClipboardText(text)
	if err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// SetClipboardData needs a real owner after EmptyClipboard. Use a hidden,
	// message-only window on this thread; OpenClipboard(NULL) is read-only here.
	class, _ := windows.UTF16PtrFromString("STATIC")
	const messageOnlyWindow = ^uintptr(2) // HWND_MESSAGE (-3)
	owner, _, err := clipboardCreateWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0, 0, 0, 0, 0, messageOnlyWindow, 0, 0, 0)
	if owner == 0 {
		return fmt.Errorf("cannot create clipboard owner: %w", err)
	}
	defer clipboardDestroyWindow.Call(owner)
	const movableZeroed = 0x0042 // GMEM_MOVEABLE | GMEM_ZEROINIT
	handle, _, err := clipboardGlobalAlloc.Call(movableZeroed, uintptr(len(data)))
	if handle == 0 {
		return fmt.Errorf("cannot allocate clipboard text: %w", err)
	}
	defer func() {
		if handle != 0 {
			clipboardGlobalFree.Call(handle)
		}
	}()
	address, _, err := clipboardGlobalLock.Call(handle)
	if address == 0 {
		return fmt.Errorf("cannot access clipboard allocation: %w", err)
	}
	err = windows.WriteProcessMemory(windows.CurrentProcess(), address, &data[0], uintptr(len(data)), nil)
	clipboardGlobalUnlock.Call(handle)
	if err != nil {
		return fmt.Errorf("cannot prepare clipboard text: %w", err)
	}
	if err := lockWindowsClipboard(owner); err != nil {
		return err
	}
	defer closeClipboard.Call()
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("cannot replace clipboard contents: %w", err)
	}
	if result, _, err := setClipboardData.Call(unicodeClipboardText, handle); result == 0 {
		return fmt.Errorf("cannot set clipboard text: %w", err)
	}
	handle = 0 // Ownership has transferred to Windows; do not free this memory.
	return nil
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
