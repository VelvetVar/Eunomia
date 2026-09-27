package eunomia

import (
	"fmt"
	"runtime"

	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
)

type keyHint struct{ Key, Action string }

var (
	deviceKeys = []keyHint{
		{"Enter", "Connect"}, {"e", "Edit"}, {"Delete", "Remove"},
		{"Shift+M", "Move menu"}, {"Alt+↑/↓", "Move up/down"}, {"→", "Choose folder"},
		{"v", "Details"}, {"f", "Forget fingerprint"},
	}
	folderKeys = []keyHint{
		{"Shift+M", "Move folder"}, {"Alt+↑/↓", "Move folder up/down"},
		{"Enter/Space", "Expand/collapse"}, {"←/→", "Collapse/expand"},
		{"e", "Rename folder"}, {"Delete", "Delete folder; keep devices"},
	}
	labKeys = []keyHint{
		{"↑/↓", "Select"}, {"a", "Add device"}, {"Shift+F", "New folder"},
		{"/", "Search"}, {"d", "Discover"}, {"p", "Ping"}, {"r", "Reload"},
		{"m", "Animation"}, {"q/Ctrl+C", "Quit"}, {"?", "All keys"},
	}
	tabKeys  = []keyHint{{"F6", "Next tab"}, {"Shift+F6", "Previous tab"}, {"Ctrl+B", "Tab commands"}}
	editKeys = []keyHint{
		{"←/→", "Cursor"}, {"Home/End", "Start/end"}, {"Backspace/Delete", "Erase"}, {"Ctrl+U", "Clear field"},
	}
	discoverKeys = []keyHint{
		{"↑/↓", "Select"}, {"Enter/a", "Add/select device"}, {"←/→", "Network"},
		{"c", "Custom subnet"}, {"r", "Scan"}, {"x", "Stop scan"},
		{"Esc", "Lab"}, {"q/Ctrl+C", "Quit"}, {"?", "All keys"},
	}
	prefixKeys = []keyHint{
		{"n/Tab", "Next tab"}, {"p/Shift+Tab", "Previous tab"}, {"h/0", "Lab"},
		{"d", "Discover"}, {"1-9", "SSH tab"}, {"?", "All keys"}, {"Esc", "Cancel"},
	}
	sshPrefixKeys = []keyHint{
		{"x", "Close SSH tab"}, {"↑/↓", "Scroll lines"}, {"PgUp/PgDn", "Scroll pages"},
		{"u", "Page up"}, {"b/Ctrl+B", "Send Ctrl+B"},
	}
)

func (a *App) commandHints() (string, []keyHint) {
	if a.HelpOpen {
		return "KEYBOARD GUIDE", []keyHint{{"←/→", "Page"}, {"PgUp/PgDn", "Page"}, {"Esc", "Back"}}
	}
	if a.Busy {
		return "WORKING", nil
	}
	if a.Pending != nil {
		return "CONFIRM", []keyHint{{"y", "Confirm"}, {"n/Esc", "Cancel"}, {"Ctrl+C", "Cancel"}}
	}
	if a.Prefix {
		keys := append([]keyHint{}, prefixKeys...)
		if a.active() != nil {
			keys = append(keys, sshPrefixKeys...)
		}
		return "TAB COMMANDS — release Ctrl+B, then press a key", keys
	}
	context := "LAB KEYS — Shift means hold Shift"
	keys := []keyHint{}
	editing := false
	if a.View == -1 {
		context = "DISCOVER KEYS"
		if a.Scan.Editing {
			context, editing = "SUBNET ENTRY", true
			keys = append(keys, keyHint{"Enter", "Use subnet and scan"}, keyHint{"Esc", "Cancel"})
		} else {
			keys = append(keys, discoverKeys...)
		}
	} else {
		switch a.Mode {
		case "form":
			context, editing = "DEVICE FORM", true
			keys = append(keys, keyHint{"Enter", "Save device"}, keyHint{"Esc", "Cancel"}, keyHint{"Tab/↓", "Next field"}, keyHint{"Shift+Tab/↑", "Previous field"})
		case "folder":
			context, editing = "FOLDER NAME", true
			keys = append(keys, keyHint{"Enter", "Save folder"}, keyHint{"Esc", "Cancel"})
		case "search":
			context, editing = "SEARCH — type to filter devices", true
			keys = append(keys, keyHint{"Enter", "Apply search"}, keyHint{"Esc", "Clear search"})
		case "move":
			context = "MOVE DEVICE"
			if a.Move.FolderID != "" {
				context = "MOVE FOLDER AND ITS DEVICES"
			}
			keys = append(keys, keyHint{"↑/↓", "Choose"}, keyHint{"Enter", "Move"}, keyHint{"Esc", "Cancel"})
		case "details":
			context = "DEVICE DETAILS"
			keys = append(keys, keyHint{"f", "Forget fingerprint"}, keyHint{"Esc", "Back"})
		case "fingerprint":
			context = "SSH FINGERPRINT"
			keys = append(keys, keyHint{"Esc", "Cancel"})
		default:
			if _, ok := a.selectedFolder(); ok {
				context = "FOLDER KEYS — deleting keeps its devices"
				keys = append(keys, folderKeys...)
			} else if device, ok := a.selected(); ok {
				keys = append(keys, deviceKeys...)
				if a.Layout.DeviceFolders[device.ID] != "" && a.Query == "" {
					keys = append(keys, keyHint{"←", "Collapse folder"})
				}
			}
			keys = append(keys, labKeys...)
			if a.Query != "" {
				keys = append(keys, keyHint{"Esc", "Clear search"})
			}
		}
	}
	if editing {
		keys = append(keys, editKeys...)
		if runtime.GOOS == "windows" {
			keys = append(keys, keyHint{"Right-click", "Paste"})
		}
	}
	if a.Mode != "list" && a.View != -1 || a.Scan.Editing && a.View == -1 {
		keys = append(keys, keyHint{"Ctrl+C", "Quit"})
	}
	return context, append(keys, tabKeys...)
}

// Keep each shortcut intact when wrapping so its action is never clipped.
func hintRows(keys []keyHint, width int) [][]keyHint {
	rows := [][]keyHint{}
	used := 0
	for _, key := range keys {
		widthNeeded := runewidth.StringWidth(key.Key+" "+key.Action) + 2
		if len(rows) == 0 || used+widthNeeded-2 > width {
			rows = append(rows, []keyHint{})
			used = 0
		}
		rows[len(rows)-1] = append(rows[len(rows)-1], key)
		used += widthNeeded
	}
	return rows
}

func (a *App) commandBarTop(w, h int) int {
	_, keys := a.commandHints()
	return h - len(hintRows(keys, w-6)) - 2
}

func (a *App) drawCommandBar(w, h int) {
	context, keys := a.commandHints()
	rows := hintRows(keys, w-6)
	top := h - len(rows) - 2
	a.rule(top)
	message, style := context, dimStyle
	if !a.HelpOpen {
		if a.View == -1 && a.Scan.Message != "" {
			message = a.Scan.Message
		} else if a.View == 0 && a.Message != "" {
			message = a.Message
			if a.Error {
				style = redStyle
			}
		}
		if a.Pending != nil || a.pasteError != "" {
			message, style = a.prompt(), goldStyle
		} else if a.Prefix {
			message, style = context, goldStyle
		}
	}
	a.put(3, top+1, runewidth.Truncate(message, w-6, "…"), style, w-6)
	for i, row := range rows {
		x := 3
		for _, key := range row {
			a.put(x, top+2+i, key.Key, tealStyle.Bold(true), w-3-x)
			x += runewidth.StringWidth(key.Key)
			a.put(x, top+2+i, " "+key.Action, baseStyle, w-3-x)
			x += runewidth.StringWidth(" "+key.Action) + 2
		}
	}
}

func helpLines(width int) []string {
	lines := []string{}
	section := func(title string, keys []keyHint) {
		lines = append(lines, "", title)
		for _, key := range keys {
			lines = append(lines, wrapText(key.Key+"  "+key.Action, width)...)
		}
	}
	section("LAB — DEVICE SELECTED", deviceKeys)
	section("LAB — FOLDER SELECTED", folderKeys)
	section("LAB — GENERAL", labKeys)
	section("LAB — MORE", []keyHint{{"j/k or mouse wheel", "Select rows"}, {"← on a folder member", "Collapse its folder"}, {"Esc", "Clear search / return to Lab list"}})
	section("FORMS, FOLDER NAME, SEARCH, SUBNET", editKeys)
	section("EDITING — MORE", []keyHint{
		{"Tab/↓ / Shift+Tab/↑", "Next / previous device-form field"},
		{"Enter", "Save form / apply search / use subnet and scan"},
		{"Esc", "Cancel / clear search"}, {"Right-click (Windows)", "Paste in the active text field"},
		{"Ctrl+C", "Quit; cancels confirmation dialogs"},
	})
	section("MOVE MENU", []keyHint{{"↑/↓ or k/j", "Choose action"}, {"Enter", "Move device"}, {"Esc", "Cancel"}})
	section("DISCOVER", discoverKeys)
	section("TABS — FROM ANY SCREEN", tabKeys)
	section("AFTER CTRL+B — RELEASE, THEN PRESS", prefixKeys)
	section("AFTER CTRL+B — SSH TAB ONLY", sshPrefixKeys)
	section("SSH — SCROLL MODE", []keyHint{
		{"↑/↓ / PgUp/PgDn", "Scroll lines / pages"}, {"Mouse wheel", "Scroll history"},
		{"Alt+↑/↓", "Send arrows to shell"}, {"Alt+PgUp/PgDn", "Send page keys to shell"},
		{"Esc", "Return from scrollback to live output"}, {"F7", "Toggle remote selection mode"},
		{"Tab / Ctrl+C / typing", "Send to remote session"},
	})
	section("SSH — REMOTE SELECTION / FULL-SCREEN APP", []keyHint{
		{"Arrows / page keys", "Send to remote program"}, {"F7", "Toggle selection mode (main screen)"},
		{"Mouse wheel", "History; remote mouse in full-screen apps"},
		{"Ctrl+B then ?", "Open this guide without sending SSH input"},
	})
	section("SSH — COPY AND PASTE", []keyHint{
		{"Left-drag, release", "Select text and copy"}, {"Right-click with selection", "Copy only; never paste"},
		{"Right-click, no selection", "Paste on Windows"}, {"Esc with selection", "Clear selection"},
	})
	return lines[1:]
}

func (a *App) openHelp() { a.HelpOpen, a.HelpPage = true, 0 }

func (a *App) helpPageCount(w, h int) int {
	capacity := max(1, a.commandBarTop(w, h)-7)
	return (len(helpLines(w-6)) + capacity - 1) / capacity
}

func (a *App) helpKey(event *tcell.EventKey) {
	w, h := a.Screen.Size()
	switch event.Key() {
	case tcell.KeyEscape:
		a.HelpOpen = false
	case tcell.KeyRight, tcell.KeyDown, tcell.KeyPgDn, tcell.KeyTAB:
		a.HelpPage++
	case tcell.KeyLeft, tcell.KeyUp, tcell.KeyPgUp, tcell.KeyBacktab:
		a.HelpPage--
	case tcell.KeyHome:
		a.HelpPage = 0
	case tcell.KeyEnd:
		a.HelpPage = a.helpPageCount(w, h) - 1
	}
	a.HelpPage = max(0, min(a.HelpPage, a.helpPageCount(w, h)-1))
}

func (a *App) drawHelp(w, h int) {
	a.put(3, 1, fmt.Sprintf("ALL KEYS / PAGE %d OF %d", min(a.HelpPage+1, a.helpPageCount(w, h)), a.helpPageCount(w, h)), tealStyle.Bold(true), w-6)
	a.put(3, 3, "Shift+M = hold Shift and press M. Plain m = animation.", whiteStyle, w-6)
	a.put(3, 4, "Shift+F = new folder. Plain f = forget fingerprint.", whiteStyle, w-6)
	a.put(3, 5, "Keys are case-sensitive. Ctrl+B then key is a sequence.", dimStyle, w-6)
	lines := helpLines(w - 6)
	capacity := max(1, a.commandBarTop(w, h)-7)
	a.HelpPage = max(0, min(a.HelpPage, a.helpPageCount(w, h)-1))
	start := a.HelpPage * capacity
	for i := start; i < min(len(lines), start+capacity); i++ {
		a.put(3, 7+i-start, lines[i], baseStyle, w-6)
	}
	a.drawCommandBar(w, h)
}
