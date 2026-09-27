package eunomia

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
)

func benchmarkLab(size int) *App {
	layout := newLabLayout()
	devices := make([]Device, size)
	for i := range size / 10 {
		id := fmt.Sprintf("folder-%04d", i)
		layout.Folders = append(layout.Folders, Folder{ID: id, Name: id})
		layout.RootOrder = append(layout.RootOrder, "folder:"+id)
	}
	for i := range devices {
		id := fmt.Sprintf("device-%05d", i)
		parent := layout.Folders[i/10].ID
		devices[i] = Device{ID: id, Name: id, Host: "127.0.0.1", Username: "admin", Port: 22}
		layout.DeviceFolders[id] = parent
		layout.Order = append(layout.Order, id)
		layout.FolderOrder[parent] = append(layout.FolderOrder[parent], "device:"+id)
	}
	return &App{Devices: devices, Filtered: devices, Layout: layout, Mode: "list", Reach: map[string]Reachability{}}
}

func BenchmarkLabRows(b *testing.B) {
	for _, size := range []int{100, 1000} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			a := benchmarkLab(size)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if len(a.labRows()) != size+size/10 {
					b.Fatal("missing Lab entries")
				}
			}
		})
	}
}

func BenchmarkLabNormalize(b *testing.B) {
	a := benchmarkLab(1000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Layout.normalize(a.Devices)
	}
}

func BenchmarkLabDraw(b *testing.B) {
	a := benchmarkLab(1000)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	a.Screen = screen
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Draw()
	}
}

func BenchmarkSSHDraw(b *testing.B) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		b.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(120, 40)
	session := &Session{Device: fixtureDevice(), Term: vt.NewEmulator(120, 38), CursorVisible: true}
	defer session.Term.Close()
	for range 80 {
		fmt.Fprint(session.Term, "\x1b[32muser@host\x1b[0m "+strings.Repeat("output ", 12)+"\r\n")
	}
	a := &App{Screen: screen, Sessions: []*Session{session}, View: 1}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		a.Draw()
	}
}
