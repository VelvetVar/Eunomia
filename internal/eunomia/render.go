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
	lab, discover := " 0 Lab ", " D Discover "
	if a.View == 0 {
		lab = "[0 Lab]"
	}
	if a.View == -1 {
		discover = "[D Discover]"
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
		case "forget":
			return "Forget the saved fingerprint? y confirm / n cancel"
		}
	}
	if a.Prefix {
		return "Ctrl+B: n/p tabs  h lab  D discover  x close  0-9 select"
	}
	return ""
}
func (a *App) Draw() {
	w, h := a.Screen.Size()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.Screen.SetContent(x, y, ' ', nil, baseStyle)
		}
	}
	a.Screen.HideCursor()
	if s := a.active(); s != nil {
		a.drawSession(s, w, h)
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
	if prompt := a.prompt(); prompt != "" {
		a.put(2, h-2, strings.Repeat(" ", w-4), baseStyle, w-4)
		a.put(3, h-2, prompt, goldStyle, w-6)
	}
	a.Screen.Show()
}
func (a *App) drawLab(w, h int) {
	a.put(2, 1, "E / EUNOMIA", tealStyle, w-4)
	a.put(w-26, 1, "HOMELAB DIRECTORY / GO", dimStyle, 24)
	a.rule(2)
	hero := 8
	if h >= 34 {
		hero = 14
	}
	if a.Mode == "fingerprint" || (h < 34 && a.Mode != "list" && a.Mode != "search") {
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
	footer := "a add e edit Del remove f forget D discover Enter SSH ? help"
	switch a.Mode {
	case "help":
		a.put(3, top+1, "KEYBOARD GUIDE", tealStyle, w-6)
		lines := []string{"↑ ↓ / j k select         Enter connect / resume SSH", "a add / e edit          Delete remove (confirmation)", "f forget fingerprint    D discover port 22", "/ search / r reload     p ping now (auto every 60s)", "m animation             q quit from the directory", "Forms: Tab / ↑ ↓ fields; ← → cursor; Enter save; Esc cancel", "Ctrl+B then n/p or F6 / Shift+F6: switch tabs", "Ctrl+B then h/0: Lab; D: Discover; 1–9: SSH sessions", "Ctrl+B then x: close; PgUp/PgDn: scroll; b: send Ctrl+B", "SSH: Tab and Ctrl+C are forwarded to the remote shell"}
		for i, line := range lines {
			if top+3+i < h-3 {
				a.put(3, top+3+i, line, baseStyle, w-6)
			}
		}
		footer = "Esc back to devices"
	case "details":
		a.put(3, top+1, "DEVICE DETAILS", tealStyle, w-6)
		if d, ok := a.selected(); ok {
			a.put(3, top+3, d.Name, whiteStyle, w-6)
			a.put(3, top+4, fmt.Sprintf("%s@%s / SSH %d", d.Username, d.Host, d.Port), tealStyle, w-6)
			r := a.Reach[d.ID]
			a.put(3, top+5, r.Status+" "+r.Latency+"  "+r.CheckedAt.Format("15:04:05"), dimStyle, w-6)
			for i, line := range wrapText(d.Description, w-6) {
				if top+7+i < h-3 {
					a.put(3, top+7+i, line, baseStyle, w-6)
				}
			}
		}
		footer = "f forget fingerprint / Esc back"
	case "form":
		title := "ADD A DEVICE"
		if a.Form.EditID != "" {
			title = "EDIT DEVICE"
		}
		a.put(3, top+1, title, tealStyle, w-6)
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
		footer = "Tab / ↑ ↓ field  ← → cursor  Enter save  Esc cancel"
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
					if y < h-3 {
						a.put(3, y, line, dimStyle, w-6)
						y++
					}
				}
			}
		}
		footer = "Esc cancel"
		if a.Busy {
			footer = "Forgetting saved fingerprint..."
		}
	default:
		title := fmt.Sprintf("DEVICES %d / PING EVERY 60s / p refresh", len(a.Filtered))
		if a.Mode == "search" {
			title = "SEARCH / " + a.Query + "▏"
			footer = "Type to filter / Enter apply / Esc clear"
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
		visible := max(1, h-top-9)
		offset := max(0, a.Selected-visible+1)
		for i := offset; i < min(len(a.Filtered), offset+visible); i++ {
			d := a.Filtered[i]
			y := top + 4 + i - offset
			style := baseStyle
			marker := " "
			if i == a.Selected {
				style = style.Background(tcell.NewRGBColor(24, 48, 61))
				marker = "›"
			}
			a.put(3, y, strings.Repeat(" ", split-4), style, split-4)
			a.put(3, y, marker, style, 1)
			a.put(5, y, d.Name, style, nameWidth-2)
			a.put(5+nameWidth, y, d.Host, style, hostWidth-2)
			a.put(5+nameWidth+hostWidth, y, d.Username, style, statusX-5-nameWidth-hostWidth-1)
			status := a.Reach[d.ID].Status
			if status == "" {
				status = "checking"
			}
			a.put(statusX, y, status, style, 11)
		}
		if len(a.Filtered) == 0 {
			empty := "Your homelab starts here."
			hint := "Press a to add your first device."
			if a.Query != "" {
				empty = "No matching devices."
				hint = "Esc clears the search."
			}
			a.put(5, top+5, empty, whiteStyle, w-10)
			a.put(5, top+7, hint, dimStyle, w-10)
		} else {
			d, _ := a.selected()
			a.put(3, h-4, fmt.Sprintf("%d / %d / SSH %d / %s", a.Selected+1, len(a.Filtered), d.Port, d.Description), dimStyle, w-6)
			if w >= 100 {
				for y := top + 3; y < h-4; y++ {
					a.put(split, y, "│", dimStyle, 1)
				}
				a.put(split+3, top+3, "DEVICE DETAILS", goldStyle, w-split-6)
				a.put(split+3, top+5, d.Name, whiteStyle, w-split-6)
				a.put(split+3, top+6, d.Username+"@"+d.Host, tealStyle, w-split-6)
				a.put(split+3, top+7, fmt.Sprintf("SSH / %d", d.Port), dimStyle, w-split-6)
				a.put(split+3, top+8, a.Reach[d.ID].Status+" "+a.Reach[d.ID].Latency, dimStyle, w-split-6)
				for i, line := range wrapText(d.Description, w-split-6) {
					if top+9+i < h-4 {
						a.put(split+3, top+9+i, line, dimStyle, w-split-6)
					}
				}
			}
		}
	}
	a.rule(h - 3)
	style := dimStyle
	if a.Message != "" {
		footer = a.Message
	}
	if a.Error {
		style = redStyle
	}
	a.put(3, h-2, footer, style, w-6)
}
func inputView(text string, cursor, width int) string {
	chars := []rune(text)
	cursor = max(0, min(cursor, len(chars)))
	start := max(0, cursor-max(1, width/2))
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
	a.put(3, 6, "← → network / c custom range / r scan / x stop", dimStyle, w-6)
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
	visible := max(1, h-17)
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
	a.rule(h - 3)
	footer := "↑ ↓ select / Enter add or select / Esc Lab"
	if s.Editing {
		footer = "Enter use range / Esc cancel"
	}
	if s.Message != "" {
		footer = s.Message
	}
	a.put(3, h-2, footer, dimStyle, w-6)
}
func vtColor(c color.Color, fallback tcell.Color) tcell.Color {
	if c == nil {
		return fallback
	}
	r, g, b, _ := c.RGBA()
	return tcell.NewRGBColor(int32(r>>8), int32(g>>8), int32(b>>8))
}
func cellStyle(c *uv.Cell) tcell.Style {
	if _, off := os.LookupEnv("NO_COLOR"); off {
		return tcell.StyleDefault
	}
	s := c.Style
	return tcell.StyleDefault.Foreground(vtColor(s.Fg, tcell.ColorWhite)).Background(vtColor(s.Bg, tcell.ColorBlack)).Bold(s.Attrs&uv.AttrBold != 0).Dim(s.Attrs&uv.AttrFaint != 0).Italic(s.Attrs&uv.AttrItalic != 0).Reverse(s.Attrs&uv.AttrReverse != 0).Blink(s.Attrs&uv.AttrBlink != 0).StrikeThrough(s.Attrs&uv.AttrStrikethrough != 0).Underline(s.Underline != 0)
}
func (a *App) drawSession(s *Session, w, h int) {
	if w < 8 || h < 3 {
		return
	}
	a.put(0, 0, a.tabLabel(w), tealStyle, w)
	prompt := a.prompt()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Unread = false
	offset := min(s.Offset, s.Term.ScrollbackLen())
	if s.Term.IsAltScreen() {
		offset = 0
	}
	for y := 0; y < h-2; y++ {
		for x := 0; x < w; x++ {
			var cell *uv.Cell
			position := y - offset
			if position < 0 {
				cell = s.Term.ScrollbackCellAt(x, s.Term.ScrollbackLen()+position)
			} else {
				cell = s.Term.CellAt(x, position)
			}
			style := tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
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
			runes := []rune(text)
			a.Screen.SetContent(x, y+1, runes[0], runes[1:], style)
		}
	}
	footer := "Ctrl+B: n/p tabs | h Lab | D Discover | x close | PgUp/PgDn"
	if s.Exited {
		footer = fmt.Sprintf("Exited %d | %s", s.ExitCode, footer)
	} else if offset > 0 {
		footer = fmt.Sprintf("Scrollback -%d | %s", offset, footer)
	}
	if prompt != "" {
		footer = prompt
	}
	a.put(0, h-1, footer, dimStyle, w)
	if offset == 0 && prompt == "" && !s.Exited && s.CursorVisible {
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
	for _, p := range logoMesh {
		xx := p.x*math.Cos(angle) + p.z*math.Sin(angle)
		zz := p.z*math.Cos(angle) - p.x*math.Sin(angle)
		yy := p.y*math.Cos(tilt) - zz*math.Sin(tilt)
		zzz := p.y*math.Sin(tilt) + zz*math.Cos(tilt)
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
