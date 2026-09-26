package eunomia

import (
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/gdamore/tcell/v2"
)

// A selection owns a snapshot of the visible SSH content. Incoming output keeps
// flowing to the emulator without changing the text beneath the user's mouse.
type terminalSelection struct {
	session       *Session
	width, height int
	cells         []uv.Cell
	anchor, end   int
	dragging      bool
	moved         bool
	status        string
}

// Called with s.mu held. y is relative to the SSH content, below the tab bar.
func sessionViewCell(s *Session, x, y int) *uv.Cell {
	offset := min(s.Offset, s.Term.ScrollbackLen())
	if s.Term.IsAltScreen() {
		offset = 0
	}
	if position := y - offset; position < 0 {
		return s.Term.ScrollbackCellAt(x, s.Term.ScrollbackLen()+position)
	} else {
		return s.Term.CellAt(x, position)
	}
}

func newTerminalSelection(s *Session, width, height, x, y int) *terminalSelection {
	selection := &terminalSelection{session: s, width: width, height: height, cells: make([]uv.Cell, width*height), dragging: true}
	s.mu.Lock()
	defer s.mu.Unlock()
	for row := 0; row < height; row++ {
		for column := 0; column < width; column++ {
			cell := sessionViewCell(s, column, row)
			if cell == nil {
				selection.cells[row*width+column] = uv.EmptyCell
			} else {
				selection.cells[row*width+column] = *cell
			}
		}
	}
	selection.anchor = selection.position(x, y)
	selection.end = selection.anchor
	return selection
}

func (s *terminalSelection) position(x, y int) int {
	x, y = max(0, min(s.width-1, x)), max(0, min(s.height-1, y))
	index := y*s.width + x
	// A wide grapheme's trailing cells belong to its leading cell.
	for x > 0 && s.cells[index].Width == 0 {
		x--
		index--
	}
	return index
}

func (s *terminalSelection) extend(x, y int) {
	s.end = s.position(x, y)
	s.moved = s.moved || s.end != s.anchor
}

func (s *terminalSelection) contains(x, y int) bool {
	position := s.position(x, y)
	return position >= min(s.anchor, s.end) && position <= max(s.anchor, s.end)
}

func (s *terminalSelection) text() string {
	start, end := min(s.anchor, s.end), max(s.anchor, s.end)
	var text strings.Builder
	for row := start / s.width; row <= end/s.width; row++ {
		var line strings.Builder
		for index := max(start, row*s.width); index <= min(end, (row+1)*s.width-1); index++ {
			cell := s.cells[index]
			if cell.Width == 0 {
				continue
			}
			if cell.Content == "" || cell.Style.Attrs&uv.AttrConceal != 0 {
				line.WriteString(strings.Repeat(" ", max(1, cell.Width)))
			} else {
				line.WriteString(cell.Content)
			}
		}
		if row > start/s.width {
			text.WriteByte('\n')
		}
		// Drop terminal padding at line ends while keeping indentation.
		text.WriteString(strings.TrimRight(line.String(), " "))
	}
	return text.String()
}

func (a *App) selectionMouse(event *tcell.EventMouse, previous tcell.ButtonMask) bool {
	s := a.active()
	if a.selection != nil && a.selection.session != s {
		a.selection = nil
	}
	if s == nil {
		return false
	}
	x, y := event.Position()
	w, h := a.Screen.Size()
	if w < 8 || h < 3 {
		a.selection = nil
		return false
	}
	buttons := event.Buttons()
	if buttons&(tcell.WheelUp|tcell.WheelDown|tcell.WheelLeft|tcell.WheelRight) != 0 {
		a.selection = nil
		return false
	}
	if a.selection != nil && a.selection.dragging {
		a.selection.extend(x, y-1)
		if buttons&tcell.ButtonPrimary == 0 {
			a.selection.dragging = false
			if !a.selection.moved {
				a.selection = nil
			} else {
				a.copySelection()
			}
		}
		return true
	}
	if buttons&tcell.ButtonPrimary != 0 && previous&tcell.ButtonPrimary == 0 && x >= 0 && x < w && y >= 1 && y < h-1 {
		a.pasteError = ""
		a.selection = newTerminalSelection(s, w, h-2, x, y-1)
		return true
	}
	return false
}

func (a *App) copySelection() {
	text := a.selection.text()
	if text == "" {
		a.selection = nil
		return
	}
	if a.writeClipboard == nil {
		a.Screen.SetClipboard([]byte(text))
		a.selection.status = "Copy requested"
	} else if err := a.writeClipboard(text); err != nil {
		a.selection.status = "Copy failed: " + safe(err.Error())
	} else {
		a.selection.status = "Copied selection"
	}
}
