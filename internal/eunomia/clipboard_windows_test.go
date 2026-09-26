//go:build windows

package eunomia

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
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
