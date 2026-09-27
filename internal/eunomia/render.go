package eunomia

import (
	"fmt"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"image/color"
	"math"
	"os"
	"strings"
)

var (
	background = tcell.NewRGBColor(8, 14, 23)
	baseStyle  = tcell.StyleDefault.Foreground(tcell.NewRGBColor(208, 219, 236)).Background(background)
	dimStyle   = baseStyle.Foreground(tcell.NewRGBColor(113, 133, 159))
	tealStyle  = baseStyle.Foreground(tcell.NewRGBColor(94, 234, 212))
	goldStyle  = baseStyle.Foreground(tcell.NewRGBColor(231, 193, 116))
	redStyle   = baseStyle.Foreground(tcell.NewRGBColor(251, 128, 139))
	whiteStyle = baseStyle.Foreground(tcell.NewRGBColor(240, 246, 255))
)

func (a *App) put(x, y int, text string, style tcell.Style, width int) {
	w, h := a.Screen.Size()
	if y < 0 || y >= h {
		return
	}
	if _, off := os.LookupEnv("NO_COLOR"); off {
		_, _, attrs := style.Decompose()
		style = tcell.StyleDefault.Attributes(attrs)
	}
	end := min(w, x+max(0, width))
	for _, r := range safe(text) {
		size := runewidth.RuneWidth(r)
		if size == 0 {
			continue
		}
		if x+size > end {
			break
		}
		if x >= 0 {
			a.Screen.SetContent(x, y, r, nil, style)
		}
		x += size
	}
}
func (a *App) rule(y int) {
	w, _ := a.Screen.Size()
	a.put(2, y, strings.Repeat("─", max(0, w-4)), dimStyle, w-4)
}
func (a *App) tabLabel(w int) string {
	lab, discover := " 0 Lab ", " d Discover "
	if a.View == 0 {
		lab = "[0 Lab]"
	}
	if a.View == -1 {
		discover = "[d Discover]"
	}
	label := lab + " " + discover
	start := 0
	if a.View > 0 {
		start = max(0, a.View-1-max(0, (w-38)/23-1))
	}
	if start > 0 {
		label += fmt.Sprintf(" <%d", start)
	}
	for i := start; i < len(a.Sessions); i++ {
		s := a.Sessions[i]
		s.mu.Lock()
		name := runewidth.Truncate(s.Device.Name, 16, "")
		suffix := ""
		if s.Exited {
			suffix = " !"
		} else if s.Unread && a.View != i+1 {
			suffix = " *"
		}
		s.mu.Unlock()
		tab := fmt.Sprintf(" %d %s%s ", i+1, name, suffix)
		if a.View == i+1 {
			tab = "[" + strings.TrimSpace(tab) + "]"
		}
		if runewidth.StringWidth(label+tab) > w-4 && i > start {
			label += fmt.Sprintf(" +%d", len(a.Sessions)-i)
			break
		}
		label += " " + tab
	}
	return label
}
func (a *App) prompt() string {
	if a.Pending != nil {
		switch a.Pending.Kind {
		case "quit":
			return fmt.Sprintf("Disconnect %d active sessions and quit? y / n", a.running())
		case "close":
			return "Disconnect this SSH session? y / n"
		case "delete":
			return "Remove " + a.Pending.Device.Name + "? y confirm / n cancel"
		case "delete-folder":
			return "Delete " + runewidth.Truncate(a.Pending.Folder.Name, 16, "…") + "? Keep devices/subfolders. y / n"
		case "forget":
			return "Forget the saved fingerprint? y confirm / n cancel"
		}
	}
	if a.Prefix {
		return "After Ctrl+B: n/p tabs  h Lab  d Discover  ? all keys"
	}
	if a.pasteError != "" {
		return a.pasteError
	}
	return ""
}
func (a *App) Draw() {
	w, h := a.Screen.Size()
	clearStyle := baseStyle
	if _, off := os.LookupEnv("NO_COLOR"); off {
		clearStyle = tcell.StyleDefault
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.Screen.SetContent(x, y, ' ', nil, clearStyle)
		}
	}
	a.Screen.HideCursor()
	if a.HelpOpen {
		a.drawHelp(w, h)
		a.Screen.Show()
		return
	}
	if s := a.active(); s != nil {
		a.drawSession(s, w, h)
		if (a.Prefix || a.Pending != nil) && w >= 64 && h >= 24 {
			for y := a.commandBarTop(w, h); y < h; y++ {
				a.put(0, y, strings.Repeat(" ", w), baseStyle, w)
			}
			a.drawCommandBar(w, h)
		}
		a.Screen.Show()
		return
	}
	if w < 64 || h < 24 {
		a.put(1, 1, "EUNOMIA", tealStyle, w-2)
		a.put(1, 3, "Resize terminal to at least 64 x 24. Press q to quit.", baseStyle, w-2)
		a.Screen.Show()
		return
	}
	a.put(2, 0, a.tabLabel(w-4), tealStyle, w-4)
	if a.View == -1 {
		a.drawDiscovery(w, h)
	} else {
		a.drawLab(w, h)
	}
	a.drawCommandBar(w, h)
	a.Screen.Show()
}
func (a *App) drawLab(w, h int) {
	bottom := a.commandBarTop(w, h)
	a.put(2, 1, "E / EUNOMIA", tealStyle, w-4)
	a.put(w-26, 1, "HOMELAB DIRECTORY / GO", dimStyle, 24)
	a.rule(2)
	hero := 8
	if h >= 34 {
		hero = 14
	}
	if a.Mode == "fingerprint" || a.Mode == "move" || a.Mode == "folder" || (h < 34 && a.Mode != "list" && a.Mode != "search") || hero+13 > bottom {
		hero = 0
	}
	logoWidth := 26
	if w >= 90 {
		logoWidth = 39
	}
	if hero > 0 {
		for y, line := range Logo(logoWidth, hero, a.Angle) {
			a.put(2, 3+y, line, goldStyle, logoWidth)
			for x, r := range line {
				if r == '@' || r == '*' {
					a.put(2+x, 3+y, string(r), tealStyle, 1)
				}
			}
		}
		a.put(logoWidth+4, 6, "E U N O M I A", whiteStyle, w-logoWidth-6)
		a.put(logoWidth+4, 8, "Every device. One place.", baseStyle, w-logoWidth-6)
		if hero > 8 {
			a.put(logoWidth+4, 10, "A little order for the things you build.", dimStyle, w-logoWidth-6)
			reachable := 0
			for _, d := range a.Devices {
				if a.Reach[d.ID].Status == "reachable" {
					reachable++
				}
			}
			a.put(logoWidth+4, 13, fmt.Sprintf("%d SAVED / %d REACHABLE / PING 60s", len(a.Devices), reachable), goldStyle, w-logoWidth-6)
		}
	}
	top := hero + 4
	a.rule(top)
	switch a.Mode {
	case "folder":
		a.drawFolderForm(w, top)
	case "move":
		a.drawMove(w, bottom, top)
	case "details":
		a.put(3, top+1, "DEVICE DETAILS", tealStyle, w-6)
		if d, ok := a.selected(); ok {
			a.put(3, top+3, d.Name, whiteStyle, w-6)
			a.put(3, top+4, fmt.Sprintf("%s@%s / SSH %d", d.Username, d.Host, d.Port), tealStyle, w-6)
			r := a.Reach[d.ID]
			a.put(3, top+5, r.Status+" "+r.Latency+"  "+r.CheckedAt.Format("15:04:05"), dimStyle, w-6)
			a.put(3, top+6, a.deviceLocation(d.ID), dimStyle, w-6)
			for i, line := range wrapText(d.Description, w-6) {
				if top+7+i < bottom {
					a.put(3, top+7+i, line, baseStyle, w-6)
				}
			}
		}
	case "form":
		title := "ADD A DEVICE"
		if a.Form.EditID != "" {
			title = "EDIT DEVICE"
		}
		a.put(3, top+1, title, tealStyle, w-6)
		if folder, err := a.Layout.folder(a.Form.FolderID); err == nil {
			a.put(3, top+2, "Folder: "+folder.Name, dimStyle, w-6)
		}
		labels := []string{"Device name", "IP address / hostname", "Default user", "SSH port", "Description"}
		for i, label := range labels {
			style := dimStyle
			marker := " "
			value := a.Form.Values[i]
			if a.Form.Field == i {
				style = tealStyle
				marker = "›"
				value = inputView(value, a.Form.Cursor, w-34)
			}
			a.put(3, top+3+i, marker+" "+label, style, 26)
			a.put(30, top+3+i, value, whiteStyle, w-33)
		}
	case "fingerprint":
		a.put(3, top+1, "FORGET SAVED SSH FINGERPRINT", goldStyle, w-6)
		if a.keyPlan == nil {
			a.put(3, top+3, "Reading SSH host-key configuration...", baseStyle, w-6)
		} else {
			p := a.keyPlan
			a.put(3, top+3, p.Device.Name+" / "+p.Target, whiteStyle, w-6)
			a.put(3, top+5, fmt.Sprintf("Remove this host's keys from %d user known_hosts files.", len(p.Files)), baseStyle, w-6)
			a.put(3, top+7, "Reconnect to verify the replacement fingerprint.", dimStyle, w-6)
			a.put(3, top+8, "Existing sessions and saved profiles stay intact.", dimStyle, w-6)
			y := top + 10
			for _, file := range p.Files {
				for _, line := range wrapText(file, w-6) {
					if y < bottom {
						a.put(3, y, line, dimStyle, w-6)
						y++
					}
				}
			}
		}
	default:
		rows := a.labRows()
		title := fmt.Sprintf("DEVICES %d / PING EVERY 60s / p refresh", len(a.Filtered))
		if a.Mode == "search" {
			title = "SEARCH / " + inputView(a.Query, a.searchCursor, w-15)
		}
		a.put(3, top+1, title, tealStyle, w-6)
		split := w - 2
		if w >= 100 {
			split = w * 62 / 100
		}
		nameWidth := (split - 19) * 39 / 100
		hostWidth := (split - 19) * 43 / 100
		statusX := split - 12
		a.put(5, top+3, "NAME", dimStyle, nameWidth-1)
		a.put(5+nameWidth, top+3, "ADDRESS", dimStyle, hostWidth-1)
		a.put(5+nameWidth+hostWidth, top+3, "USER", dimStyle, statusX-5-nameWidth-hostWidth)
		a.put(statusX, top+3, "PING", dimStyle, 11)
		visible := max(1, bottom-top-5)
		offset := max(0, a.Selected-visible+1)
		for i := offset; i < min(len(rows), offset+visible); i++ {
			row := rows[i]
			d := row.Device
			y := top + 4 + i - offset
			style := baseStyle
			marker := " "
			if i == a.Selected {
				style = style.Background(tcell.NewRGBColor(24, 48, 61))
				marker = "›"
			}
			a.put(3, y, strings.Repeat(" ", split-4), style, split-4)
			a.put(3, y, marker, style, 1)
			if row.IsFolder {
				symbol := "[-] "
				if row.Folder.Collapsed {
					symbol = "[+] "
				}
				indent := strings.Repeat(" ", min(4*row.Depth, max(0, split-25)))
				label := fmt.Sprintf("%s%s%s (%d)", indent, symbol, row.Folder.Name, row.Count)
				a.put(5, y, label, style.Foreground(tcell.NewRGBColor(94, 234, 212)), split-7)
				continue
			}
			name := d.Name
			if row.Depth > 0 {
				name = strings.Repeat(" ", min(4*row.Depth, max(0, nameWidth-7))) + name
			}
			a.put(5, y, name, style, nameWidth-2)
			a.put(5+nameWidth, y, d.Host, style, hostWidth-2)
			a.put(5+nameWidth+hostWidth, y, d.Username, style, statusX-5-nameWidth-hostWidth-1)
			status := a.Reach[d.ID].Status
			if status == "" {
				status = "checking"
			}
			a.put(statusX, y, status, style, 11)
		}
		if len(rows) == 0 {
			empty := "Your homelab starts here."
			hint := "Press a to add your first device."
			if a.Query != "" {
				empty = "No matching devices."
				hint = "Esc clears the search."
			}
			a.put(5, top+5, empty, whiteStyle, w-10)
			a.put(5, top+7, hint, dimStyle, w-10)
		} else if folder, ok := a.selectedFolder(); ok {
			row := rows[a.Selected]
			a.put(3, bottom-1, a.folderSummary(row), dimStyle, w-6)
			if w >= 100 {
				a.put(split+3, top+3, "FOLDER", goldStyle, w-split-6)
				a.put(split+3, top+5, folder.Name, whiteStyle, w-split-6)
				a.put(split+3, top+7, "Enter: collapse / expand", dimStyle, w-split-6)
				a.put(split+3, top+8, "a: add device / e: rename", dimStyle, w-split-6)
				a.put(split+3, top+9, "Shift+F: new subfolder", dimStyle, w-split-6)
			}
		} else {
			d, _ := a.selected()
			a.put(3, bottom-1, fmt.Sprintf("%d / %d / %s / SSH %d / %s", a.Selected+1, len(rows), a.deviceLocation(d.ID), d.Port, d.Description), dimStyle, w-6)
			if w >= 100 {
				for y := top + 3; y < bottom-1; y++ {
					a.put(split, y, "│", dimStyle, 1)
				}
				a.put(split+3, top+3, "DEVICE DETAILS", goldStyle, w-split-6)
				a.put(split+3, top+5, d.Name, whiteStyle, w-split-6)
				a.put(split+3, top+6, d.Username+"@"+d.Host, tealStyle, w-split-6)
				a.put(split+3, top+7, fmt.Sprintf("SSH / %d", d.Port), dimStyle, w-split-6)
				a.put(split+3, top+8, a.Reach[d.ID].Status+" "+a.Reach[d.ID].Latency, dimStyle, w-split-6)
				a.put(split+3, top+9, a.deviceLocation(d.ID), dimStyle, w-split-6)
				for i, line := range wrapText(d.Description, w-split-6) {
					if top+10+i < bottom-1 {
						a.put(split+3, top+10+i, line, dimStyle, w-split-6)
					}
				}
			}
		}
	}
}
func inputView(text string, cursor, width int) string {
	chars := []rune(text)
	cursor = max(0, min(cursor, len(chars)))
	start, used := cursor, 0
	for start > 0 {
		size := runewidth.RuneWidth(chars[start-1])
		if used+size > max(0, (width-1)/2) {
			break
		}
		start--
		used += size
	}
	return string(chars[start:cursor]) + "▏" + string(chars[cursor:])
}
func wrapText(text string, width int) []string {
	if width < 1 {
		return nil
	}
	result := []string{}
	var line strings.Builder
	length := 0
	for _, r := range safe(text) {
		size := runewidth.RuneWidth(r)
		if length+size > width {
			result = append(result, line.String())
			line.Reset()
			length = 0
		}
		line.WriteRune(r)
		length += size
	}
	if line.Len() > 0 {
		result = append(result, line.String())
	}
	return result
}
func (a *App) drawDiscovery(w, h int) {
	bottom := a.commandBarTop(w, h)
	s := &a.Scan
	a.put(2, 2, "DISCOVER / SSH ON YOUR NETWORK", whiteStyle, w-4)
	a.rule(3)
	label := "Press c to enter a subnet"
	if s.RangeIndex < len(s.Ranges) {
		r := s.Ranges[s.RangeIndex]
		label = r.CIDR + " / " + r.Name
	}
	if s.Editing {
		label = inputView(s.Input, s.Cursor, w-15)
	}
	a.put(3, 5, "Range: "+label, tealStyle, w-6)
	status := "READY"
	if s.HasRun {
		status = "LAST SCAN"
	}
	if s.Running {
		status = "SCANNING"
	}
	a.put(3, 8, fmt.Sprintf("%s %s  %d/%d / %d found", status, s.CIDR, s.Checked, s.Total, len(s.Results)), goldStyle, w-6)
	a.put(3, 9, "Port 22 / 24 concurrent / 0.9s deadline / no login attempts", dimStyle, w-6)
	a.rule(10)
	a.put(5, 11, "ADDRESS", dimStyle, 17)
	a.put(24, 11, "SERVICE", dimStyle, 17)
	a.put(42, 11, "PROFILE / BANNER", dimStyle, w-45)
	visible := max(1, bottom-12)
	offset := max(0, s.Selected-visible+1)
	for i := offset; i < min(len(s.Results), offset+visible); i++ {
		f := s.Results[i]
		y := 12 + i - offset
		style := baseStyle
		if i == s.Selected {
			style = style.Background(tcell.NewRGBColor(24, 48, 61))
		}
		a.put(3, y, strings.Repeat(" ", w-6), style, w-6)
		a.put(5, y, f.Host, style, 17)
		service := "Open, unverified"
		if f.Confirmed {
			service = "SSH"
		}
		a.put(24, y, service, style, 17)
		note := f.Banner
		if note == "" {
			note = "No SSH banner received"
		}
		for _, d := range a.Devices {
			if d.Host == f.Host && d.Port == 22 {
				note = "Saved: " + d.Name
				break
			}
		}
		a.put(42, y, note, style, w-45)
	}
	if len(s.Results) == 0 {
		message := "Choose a subnet, then press r to scan."
		if s.Running {
			message = "Looking for SSH services..."
		} else if s.HasRun {
			message = "No open SSH ports found. Try another range or rescan."
		}
		a.put(5, 14, message, dimStyle, w-10)
	}
}
func vtColor(c color.Color, fallback tcell.Color) tcell.Color {
	if c == nil {
		return fallback
	}
	r, g, b, _ := c.RGBA()
	return tcell.NewRGBColor(int32(r>>8), int32(g>>8), int32(b>>8))
}
func cellStyle(c *uv.Cell) tcell.Style {
	s := c.Style
	style := tcell.StyleDefault
	if _, off := os.LookupEnv("NO_COLOR"); !off {
		style = style.Foreground(vtColor(s.Fg, tcell.ColorWhite)).Background(vtColor(s.Bg, tcell.ColorBlack))
	}
	return style.Bold(s.Attrs&uv.AttrBold != 0).Dim(s.Attrs&uv.AttrFaint != 0).Italic(s.Attrs&uv.AttrItalic != 0).Reverse(s.Attrs&uv.AttrReverse != 0).Blink(s.Attrs&uv.AttrBlink != 0).StrikeThrough(s.Attrs&uv.AttrStrikethrough != 0).Underline(s.Underline != 0)
}
func (a *App) drawSession(s *Session, w, h int) {
	if w < 8 || h < 3 {
		return
	}
	a.put(0, 0, a.tabLabel(w), tealStyle, w)
	prompt := a.prompt()
	selection := a.selection
	if selection != nil && (selection.session != s || selection.width != w || selection.height != h-2) {
		a.selection = nil
		selection = nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Unread = false
	offset := min(s.Offset, s.Term.ScrollbackLen())
	if s.Term.IsAltScreen() {
		offset = 0
	}
	for y := 0; y < h-2; y++ {
		for x := 0; x < w; x++ {
			cell := sessionViewCell(s, x, y)
			if selection != nil {
				cell = &selection.cells[y*w+x]
			}
			style := cellStyle(&uv.Cell{})
			text := " "
			if cell != nil {
				if cell.Width == 0 {
					continue
				}
				style = cellStyle(cell)
				text = cell.Content
				if text == "" {
					text = " "
				}
				if cell.Style.Attrs&uv.AttrConceal != 0 {
					text = " "
				}
			}
			if selection != nil && selection.contains(x, y) {
				_, _, attrs := style.Decompose()
				style = style.Reverse(attrs&tcell.AttrReverse == 0)
			}
			runes := []rune(text)
			a.Screen.SetContent(x, y+1, runes[0], runes[1:], style)
		}
	}
	footer := "SCROLL: ↑/↓ history | F7 remote keys | Ctrl+B then ? keys"
	if w >= 90 {
		footer += " | Alt+↑/↓ shell"
	}
	if s.RemoteKeys {
		footer = "SELECT: ↑/↓ remote options | F7 scroll | Ctrl+B then ? keys"
	}
	if s.Term.IsAltScreen() {
		footer = "REMOTE APP: keys go to SSH | Ctrl+B then ? all keys"
	}
	if s.Exited {
		footer = fmt.Sprintf("Exited %d | ↑/↓ scroll | Esc bottom | Ctrl+B then x close", s.ExitCode)
	} else if offset > 0 {
		footer = fmt.Sprintf("Scrollback -%d | ↑/↓ lines | PgUp/PgDn pages | Esc live", offset)
		if s.RemoteKeys {
			footer = fmt.Sprintf("Scrollback -%d | SELECT: ↑/↓ remote | Esc live", offset)
		}
	}
	if s.Failure != "" {
		footer = safe(s.Failure) + " | Ctrl+B then x to close"
	}
	if selection != nil {
		footer = "Selecting text | release to copy | Esc cancels"
		if !selection.dragging {
			footer = selection.status + " | right-click copies | Esc clears selection"
		}
	}
	if prompt != "" {
		footer = prompt
	}
	a.put(0, h-1, footer, dimStyle, w)
	if selection == nil && offset == 0 && prompt == "" && !s.Exited && s.CursorVisible {
		cursor := s.Term.CursorPosition()
		a.Screen.ShowCursor(min(w-1, cursor.X), min(h-2, cursor.Y+1))
	}
}

type logoPoint struct {
	x, y, z float64
	core    bool
}

var logoMesh = makeLogo()

func makeLogo() []logoPoint {
	mesh := []logoPoint{}
	for ring := 0; ring < 3; ring++ {
		tilt := float64(ring) * math.Pi / 3
		for a := 0.; a < math.Pi*2; a += .024 {
			for tube := 0.; tube < math.Pi*2; tube += math.Pi / 3 {
				radius := 1.75 + .065*math.Cos(tube)
				x, y, z := radius*math.Cos(a), radius*math.Sin(a), .065*math.Sin(tube)
				mesh = append(mesh, logoPoint{x, y*math.Cos(tilt) - z*math.Sin(tilt), y*math.Sin(tilt) + z*math.Cos(tilt), false})
			}
		}
	}
	vertices := [][3]float64{{0, .82, 0}, {0, -.82, 0}, {.65, 0, 0}, {-.65, 0, 0}, {0, 0, .65}, {0, 0, -.65}}
	for i := range vertices {
		for j := i + 1; j < len(vertices); j++ {
			if i/2 == j/2 {
				continue
			}
			for t := 0.; t <= 1; t += .025 {
				a, b := vertices[i], vertices[j]
				mesh = append(mesh, logoPoint{a[0] + (b[0]-a[0])*t, a[1] + (b[1]-a[1])*t, a[2] + (b[2]-a[2])*t, true})
			}
		}
	}
	return mesh
}
func Logo(w, h int, angle float64) []string {
	if w < 1 || h < 1 {
		return nil
	}
	cells := make([][]rune, h)
	depth := make([][]float64, h)
	for y := 0; y < h; y++ {
		cells[y] = []rune(strings.Repeat(" ", w))
		depth[y] = make([]float64, w)
		for x := range depth[y] {
			depth[y][x] = math.Inf(-1)
		}
	}
	scale := min(float64(w)/8.8, float64(h)/4.5)
	tilt := .45 + .22*math.Sin(angle)
	sinAngle, cosAngle := math.Sincos(angle)
	sinTilt, cosTilt := math.Sincos(tilt)
	for _, p := range logoMesh {
		xx := p.x*cosAngle + p.z*sinAngle
		zz := p.z*cosAngle - p.x*sinAngle
		yy := p.y*cosTilt - zz*sinTilt
		zzz := p.y*sinTilt + zz*cosTilt
		perspective := 9 / (9 - zzz)
		x := int(math.Round(float64(w-1)/2 + xx*scale*2*perspective))
		y := int(math.Round(float64(h-1)/2 - yy*scale*perspective))
		if y < 0 || y >= h || x < 0 || x >= w || zzz < depth[y][x] {
			continue
		}
		depth[y][x] = zzz
		ch := '.'
		switch {
		case p.core:
			ch = '*'
			if zzz > 0 {
				ch = '@'
			}
		case zzz > .9:
			ch = '#'
		case zzz > .15:
			ch = '+'
		case zzz > -.65:
			ch = '='
		}
		cells[y][x] = ch
	}
	result := make([]string, h)
	for i, row := range cells {
		result[i] = string(row)
	}
	return result
}
