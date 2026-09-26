//go:build windows

package eunomia

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestDecodeClipboardText(t *testing.T) {
	for _, text := range []string{"", " Dummy P@ss! ", "é🔑\r\n\tsecond line"} {
		units := append(utf16.Encode([]rune(text)), 0, 'x')
		data := make([]byte, 2*len(units))
		for i, unit := range units {
			binary.LittleEndian.PutUint16(data[2*i:], unit)
		}
		got, err := decodeClipboardText(data)
		if err != nil || got != text {
			t.Fatal("clipboard text changed", err)
		}
	}
	for _, data := range [][]byte{nil, {1}, {'x', 0}} {
		if _, err := decodeClipboardText(data); err == nil {
			t.Fatal("accepted malformed clipboard text")
		}
	}
}

func TestEncodeClipboardText(t *testing.T) {
	for _, text := range []string{"", "  spaced text  ", "界e\u0301🔑\nsecond\r\nthird"} {
		data, err := encodeClipboardText(text)
		if err != nil {
			t.Fatal(err)
		}
		got, err := decodeClipboardText(data)
		want := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", "\r\n")
		if err != nil || got != want {
			t.Fatal("clipboard encoding changed Unicode or whitespace", err)
		}
	}
	for _, text := range []string{"before\x00after", strings.Repeat("x", 1<<20)} {
		if _, err := encodeClipboardText(text); err == nil {
			t.Fatal("clipboard encoding accepted invalid or oversized text")
		}
	}
}

func TestWindowsClipboardRoundTrip(t *testing.T) {
	if os.Getenv("EUNOMIA_CLIPBOARD_TEST_HELPER") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestWindowsClipboardRoundTrip$", "-test.v")
		cmd.Env = append(os.Environ(), "EUNOMIA_CLIPBOARD_TEST_HELPER=1")
		quietCommand(cmd)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("isolated clipboard test: %v\n%s", err, output)
		}
		if strings.Contains(string(output), "ISOLATION_UNAVAILABLE") {
			t.Skip(string(output))
		}
		return
	}
	// A window station has its own clipboard. This subprocess creates a fresh,
	// noninteractive station so the test never reads or replaces user clipboard
	// contents and never changes the visible desktop.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	getStation := clipboardUser32.NewProc("GetProcessWindowStation")
	setStation := clipboardUser32.NewProc("SetProcessWindowStation")
	getDesktop := clipboardUser32.NewProc("GetThreadDesktop")
	setDesktop := clipboardUser32.NewProc("SetThreadDesktop")
	originalStation, _, _ := getStation.Call()
	originalDesktop, _, _ := getDesktop.Call(uintptr(windows.GetCurrentThreadId()))
	const createOnly, genericAll = 1, 0x10000000
	stationName, _ := windows.UTF16PtrFromString(fmt.Sprintf("EunomiaClipboardTest_%d_%d", os.Getpid(), time.Now().UnixNano()))
	station, _, err := clipboardUser32.NewProc("CreateWindowStationW").Call(uintptr(unsafe.Pointer(stationName)), createOnly, genericAll, 0)
	if station == 0 {
		t.Skipf("ISOLATION_UNAVAILABLE: %v", err)
	}
	if station == originalStation {
		t.Fatal("clipboard isolation did not create a separate window station")
	}
	var desktop uintptr
	defer func() {
		setStation.Call(originalStation)
		setDesktop.Call(originalDesktop)
		if desktop != 0 {
			clipboardUser32.NewProc("CloseDesktop").Call(desktop)
		}
		clipboardUser32.NewProc("CloseWindowStation").Call(station)
	}()
	if ok, _, err := setStation.Call(station); ok == 0 {
		t.Fatal("cannot attach isolated station", err)
	}
	name, _ := windows.UTF16PtrFromString("EunomiaClipboardTest")
	desktop, _, err = clipboardUser32.NewProc("CreateDesktopW").Call(uintptr(unsafe.Pointer(name)), 0, 0, 0, genericAll, 0)
	if desktop == 0 {
		t.Fatal("cannot create isolated desktop", err)
	}
	if ok, _, err := setDesktop.Call(desktop); ok == 0 {
		t.Fatal("cannot attach isolated desktop", err)
	}
	if current, _, _ := getStation.Call(); current != station {
		t.Fatal("clipboard station changed before test")
	}
	for _, text := range []string{"Dummy copy 界e\u0301🔑\nsecond line", "replacement", ""} {
		if err := writeWindowsClipboard(text); err != nil {
			t.Fatal("native clipboard write failed", err)
		}
		got, err := readWindowsClipboard()
		if err != nil || got != strings.ReplaceAll(text, "\n", "\r\n") {
			t.Fatal("native clipboard read did not match written text", err)
		}
	}
}
