package eunomia

import (
	"context"
	"errors"
	"fmt"
	"github.com/gdamore/tcell/v2"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

type formState struct {
	Values        [5]string
	Field, Cursor int
	EditID        string
}
type action struct {
	Kind    string
	Device  Device
	Session *Session
	Plan    HostKeyPlan
}
type openedSession struct {
	Session *Session
	Err     error
}
type preparedKey struct {
	Generation int
	Plan       HostKeyPlan
	Err        error
}
type resetKey struct {
	Target string
	Err    error
}
type scanState struct {
	Ranges                                           []ScanRange
	RangeIndex, Selected, Checked, Total, Generation int
	Results                                          []Found
	Running, HasRun, Editing                         bool
	Input                                            string
	Cursor                                           int
	CIDR, Message                                    string
	Cancel                                           context.CancelFunc
}
type App struct {
	Screen               tcell.Screen
	Store                Store
	Devices, Filtered    []Device
	Selected             int
	Query, Mode, Message string
	Error                bool
	Form                 formState
	View                 int // 0 Lab, -1 Discover, 1..N SSH tabs
	Sessions             []*Session
	Prefix               bool
	Pending              *action
	Busy, Opening        bool
	Animate              bool
	Angle                float64
	Reach                map[string]Reachability
	Scan                 scanState
	ctx                  context.Context
	cancel               context.CancelFunc
	messages             chan any
	wake                 chan *Session
	sessionMarks         map[*Session]string
	pingCancel           context.CancelFunc
	keyGeneration        int
	keyPlan              *HostKeyPlan
	paste                bool
	pasteBuffer          strings.Builder
	opener               func(Device, int, int, func(*Session)) (*Session, error)
	openingWork          sync.WaitGroup
	prepare              func(context.Context, Device) (HostKeyPlan, error)
	forget               func(context.Context, HostKeyPlan, ToolRunner) error
}

func NewApp(screen tcell.Screen, store Store, animate bool) (*App, error) {
	ctx, cancel := context.WithCancel(context.Background())
	a := &App{Screen: screen, Store: store, Mode: "list", Animate: animate, Angle: .35, Reach: map[string]Reachability{}, ctx: ctx, cancel: cancel, messages: make(chan any, 256), wake: make(chan *Session, 128), sessionMarks: map[*Session]string{}, opener: OpenSession, prepare: PrepareHostKeyReset, forget: ForgetHostKey}
	a.Scan.Ranges = LocalRanges()
	if err := a.refresh(); err != nil {
		cancel()
		return nil, err
	}
	return a, nil
}
func (a *App) refresh() error {
	devices, err := a.Store.Read()
	if err != nil {
		return err
	}
	a.Devices = devices
	a.Filtered = Search(devices, a.Query)
	a.Selected = max(0, min(a.Selected, len(a.Filtered)-1))
	return nil
}
func (a *App) selected() (Device, bool) {
	if a.Selected < 0 || a.Selected >= len(a.Filtered) {
		return Device{}, false
	}
	return a.Filtered[a.Selected], true
}
func (a *App) active() *Session {
	if a.View > 0 && a.View <= len(a.Sessions) {
		return a.Sessions[a.View-1]
	}
	return nil
}
func (a *App) post(message any) {
	select {
	case a.messages <- message:
	case <-a.ctx.Done():
	}
}
func (a *App) notify(s *Session) {
	select {
	case a.wake <- s:
	default:
	}
}
func (a *App) running() int {
	count := 0
	for _, s := range a.Sessions {
		s.mu.Lock()
		if !s.Exited {
			count++
		}
		s.mu.Unlock()
	}
	return count
}
func (a *App) setMessage(err error, text string) {
	a.Error = err != nil
	if err != nil {
		a.Message = safe(err.Error())
	} else {
		a.Message = text
	}
}
func (a *App) Stop() { a.cancel() }
func (a *App) Close() {
	a.cancel()
	a.openingWork.Wait()
	// A connection may finish just as the event loop stops. Reclaim any queued
	// session whose ownership was never transferred to the tabs.
drain:
	for {
		select {
		case message := <-a.messages:
			if opened, ok := message.(openedSession); ok && opened.Session != nil {
				opened.Session.Close()
			}
		default:
			break drain
		}
	}
	if a.pingCancel != nil {
		a.pingCancel()
	}
	if a.Scan.Cancel != nil {
		a.Scan.Cancel()
	}
	for _, s := range a.Sessions {
		s.Close()
	}
}
func (a *App) Run() error {
	if err := a.Screen.Init(); err != nil {
		a.Close()
		return fmt.Errorf("interactive terminal required: %w", err)
	}
	defer a.Screen.Fini()
	defer a.Close()
	a.Screen.EnablePaste()
	a.Screen.SetStyle(baseStyle)
	a.Screen.Clear()
	control, err := StartControl(a.Store.Directory, a.Stop)
	if err != nil {
		return err
	}
	defer control.Close()
	events := make(chan tcell.Event, 64)
	go func() {
		for {
			event := a.Screen.PollEvent()
			if event == nil {
				return
			}
			select {
			case events <- event:
			case <-a.ctx.Done():
				return
			}
		}
	}()
	pingTimer := time.NewTicker(time.Minute)
	defer pingTimer.Stop()
	animation := time.NewTicker(100 * time.Millisecond)
	defer animation.Stop()
	var renderTimer *time.Timer
	var render <-chan time.Time
	dirty := false
	drawSoon := func() {
		dirty = true
		if render == nil {
			renderTimer = time.NewTimer(16 * time.Millisecond)
			render = renderTimer.C
		}
	}
	defer func() {
		if renderTimer != nil {
			renderTimer.Stop()
		}
	}()
	a.startPing()
	a.Draw()
	for {
		select {
		case <-a.ctx.Done():
			return nil
		case event := <-events:
			switch event := event.(type) {
			case *tcell.EventKey:
				a.HandleKey(event)
			case *tcell.EventResize:
				w, h := a.Screen.Size()
				for _, s := range a.Sessions {
					s.Resize(max(1, w), max(1, h-2))
				}
				a.Screen.Sync()
			case *tcell.EventPaste:
				if event.Start() {
					a.paste = true
					a.pasteBuffer.Reset()
				} else {
					a.paste = false
					if s := a.active(); s != nil {
						s.Paste(a.pasteBuffer.String())
					}
					a.pasteBuffer.Reset()
				}
			}
			drawSoon()
		case message := <-a.messages:
			a.handleMessage(message)
			drawSoon()
		case s := <-a.wake:
			s.mu.Lock()
			mark := fmt.Sprint(s.Unread, s.Exited)
			s.mu.Unlock()
			if a.active() == s || a.sessionMarks[s] != mark {
				a.sessionMarks[s] = mark
				drawSoon()
			}
		case <-pingTimer.C:
			a.startPing()
		case <-animation.C:
			if a.Animate && a.View == 0 && a.Mode == "list" {
				a.Angle += .012
				drawSoon()
			}
		case <-render:
			render = nil
			if dirty {
				a.Draw()
				dirty = false
			}
		}
	}
}
func (a *App) startPing() {
	if a.pingCancel != nil {
		a.pingCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.pingCancel = cancel
	devices := append([]Device(nil), a.Devices...)
	for _, d := range devices {
		previous := a.Reach[d.ID]
		if previous.Host != d.Host || previous.Status == "" {
			a.Reach[d.ID] = Reachability{ID: d.ID, Host: d.Host, Status: "checking"}
		}
	}
	go PingDevices(ctx, devices, func(r Reachability) { a.post(r) })
}
func (a *App) startScan() {
	s := &a.Scan
	if s.RangeIndex >= len(s.Ranges) {
		s.Message = "Press c to enter an IPv4 subnet (/24 to /32)."
		return
	}
	if s.Cancel != nil {
		s.Cancel()
	}
	s.Generation++
	ctx, cancel := context.WithCancel(a.ctx)
	s.Cancel = cancel
	s.Running = true
	s.HasRun = true
	s.Results = nil
	s.Selected = 0
	s.Checked = 0
	s.Message = ""
	s.CIDR = s.Ranges[s.RangeIndex].CIDR
	_, hosts, err := SubnetHosts(s.CIDR)
	if err != nil {
		s.Message = err.Error()
		s.Running = false
		return
	}
	s.Total = len(hosts)
	go Scan(ctx, s.CIDR, s.Generation, nil, func(update ScanUpdate) { a.post(update) })
}
func (a *App) activate(view int) {
	if a.Mode == "fingerprint" && a.Pending == nil {
		a.Mode = "list"
		a.keyGeneration++
	}
	a.View = view
	if view == -1 && !a.Scan.HasRun && len(a.Scan.Ranges) > 0 {
		a.startScan()
	}
}
func (a *App) cycle(direction int) {
	tabs := []int{0, -1}
	for i := range a.Sessions {
		tabs = append(tabs, i+1)
	}
	index := 0
	for i, v := range tabs {
		if v == a.View {
			index = i
		}
	}
	a.activate(tabs[(index+direction+len(tabs))%len(tabs)])
}
func (a *App) closeSession(target *Session) {
	for i, s := range a.Sessions {
		if s == target {
			s.Close()
			a.Sessions = append(a.Sessions[:i], a.Sessions[i+1:]...)
			if a.View == i+1 {
				a.View = 0
			} else if a.View > i+1 {
				a.View--
			}
			delete(a.sessionMarks, s)
			return
		}
	}
}
func (a *App) quit() {
	if a.running() > 0 {
		a.Pending = &action{Kind: "quit"}
	} else {
		a.Stop()
	}
}
func (a *App) handleMessage(message any) {
	switch m := message.(type) {
	case Reachability:
		for _, d := range a.Devices {
			if d.ID == m.ID && d.Host == m.Host {
				a.Reach[d.ID] = m
			}
		}
	case ScanUpdate:
		if m.Generation != a.Scan.Generation {
			return
		}
		a.Scan.Checked = m.Checked
		a.Scan.Total = m.Total
		if m.Found != nil {
			selected := ""
			if a.Scan.Selected < len(a.Scan.Results) {
				selected = a.Scan.Results[a.Scan.Selected].Host
			}
			a.Scan.Results = append(a.Scan.Results, *m.Found)
			SortFound(a.Scan.Results)
			if selected != "" {
				for i, f := range a.Scan.Results {
					if f.Host == selected {
						a.Scan.Selected = i
					}
				}
			}
		}
		if m.Done {
			a.Scan.Running = false
		}
		if m.Err != nil {
			a.Scan.Message = m.Err.Error()
		}
	case openedSession:
		a.Opening = false
		if m.Err != nil {
			a.setMessage(m.Err, "")
			return
		}
		a.Sessions = append(a.Sessions, m.Session)
		a.View = len(a.Sessions)
		w, h := a.Screen.Size()
		m.Session.Resize(max(1, w), max(1, h-2))
	case preparedKey:
		if m.Generation != a.keyGeneration || a.Mode != "fingerprint" || a.View != 0 {
			return
		}
		if m.Err != nil {
			a.Mode = "list"
			a.setMessage(m.Err, "")
			return
		}
		if len(m.Plan.Files) == 0 {
			a.Mode = "list"
			a.Message = "No matching saved fingerprint in user known_hosts files."
			return
		}
		a.keyPlan = &m.Plan
		a.Pending = &action{Kind: "forget", Plan: m.Plan}
	case resetKey:
		a.Busy = false
		a.Mode = "list"
		a.setMessage(m.Err, "Forgot "+m.Target+". Reconnect to verify its new fingerprint.")
	}
}
func (a *App) HandleKey(event *tcell.EventKey) {
	key, r := event.Key(), event.Rune()
	if a.paste && a.active() != nil {
		switch key {
		case tcell.KeyRune:
			a.pasteBuffer.WriteRune(r)
		case tcell.KeyEnter:
			a.pasteBuffer.WriteByte('\n')
		case tcell.KeyTAB:
			a.pasteBuffer.WriteByte('\t')
		}
		return
	}
	if a.Busy {
		return
	}
	if a.Pending != nil {
		pending := a.Pending
		if r == 'y' || r == 'Y' {
			a.Pending = nil
			switch pending.Kind {
			case "quit":
				a.Stop()
			case "close":
				a.closeSession(pending.Session)
			case "delete":
				err := a.Store.Remove(pending.Device.ID)
				a.Mode = "list"
				if err == nil {
					err = a.refresh()
					a.startPing()
				}
				a.setMessage(err, "Removed "+pending.Device.Name+".")
			case "forget":
				a.Busy = true
				go func() { err := a.forget(a.ctx, pending.Plan, nil); a.post(resetKey{pending.Plan.Target, err}) }()
			}
		} else if r == 'n' || r == 'N' || key == tcell.KeyEscape || key == tcell.KeyCtrlC {
			a.Pending = nil
			if pending.Kind == "forget" {
				a.Mode = "list"
			}
		}
		return
	}
	if a.Prefix {
		a.Prefix = false
		switch {
		case r == 'n' || key == tcell.KeyTAB:
			a.cycle(1)
		case r == 'p' || key == tcell.KeyBacktab:
			a.cycle(-1)
		case r == 'h' || r == '0':
			a.activate(0)
		case r == 'd' || r == 'D':
			a.activate(-1)
		case r >= '1' && r <= '9':
			index := int(r - '0')
			if index <= len(a.Sessions) {
				a.activate(index)
			}
		case r == 'x':
			if s := a.active(); s != nil {
				s.mu.Lock()
				exited := s.Exited
				s.mu.Unlock()
				if exited {
					a.closeSession(s)
				} else {
					a.Pending = &action{Kind: "close", Session: s}
				}
			}
		case r == 'u' || key == tcell.KeyPgUp:
			if s := a.active(); s != nil {
				s.Scroll(1)
			}
		case key == tcell.KeyPgDn:
			if s := a.active(); s != nil {
				s.Scroll(-1)
			}
		case r == 'b' || key == tcell.KeyCtrlB:
			if s := a.active(); s != nil {
				s.SendLiteral("\x02")
			}
		}
		return
	}
	if key == tcell.KeyCtrlB {
		a.Prefix = true
		return
	}
	if key == tcell.KeyF6 {
		direction := 1
		if event.Modifiers()&tcell.ModShift != 0 {
			direction = -1
		}
		a.cycle(direction)
		return
	}
	if s := a.active(); s != nil {
		s.SendKey(event)
		return
	}
	if key == tcell.KeyCtrlC {
		a.quit()
		return
	}
	w, h := a.Screen.Size()
	if w < 64 || h < 24 {
		if r == 'q' {
			a.quit()
		}
		return
	}
	if a.View == -1 {
		a.discoveryKey(event)
		return
	}
	if key == tcell.KeyEscape {
		a.Mode = "list"
		a.Query = ""
		a.Message = ""
		a.keyGeneration++
		a.setMessage(a.refresh(), "")
		return
	}
	if a.Mode == "form" {
		a.formKey(event)
		return
	}
	if a.Mode == "search" {
		if key == tcell.KeyEnter {
			a.Mode = "list"
		} else {
			chars := []rune(a.Query)
			cursor := len(chars)
			editText(&a.Query, &cursor, event, 100)
			a.Filtered = Search(a.Devices, a.Query)
			a.Selected = 0
		}
		return
	}
	d, hasDevice := a.selected()
	if (a.Mode == "list" || a.Mode == "details") && r == 'f' && hasDevice {
		a.Mode = "fingerprint"
		a.keyPlan = nil
		a.Message = ""
		a.keyGeneration++
		generation := a.keyGeneration
		go func() { plan, err := a.prepare(a.ctx, d); a.post(preparedKey{generation, plan, err}) }()
		return
	}
	if a.Mode != "list" {
		return
	}
	a.Message = ""
	a.Error = false
	switch {
	case r == 'q':
		a.quit()
	case r == 'm':
		a.Animate = !a.Animate
	case key == tcell.KeyDown || r == 'j':
		a.Selected = min(len(a.Filtered)-1, a.Selected+1)
	case key == tcell.KeyUp || r == 'k':
		a.Selected = max(0, a.Selected-1)
	case r == '/':
		a.Mode = "search"
	case r == '?':
		a.Mode = "help"
	case r == 'r':
		a.setMessage(a.refresh(), "")
		a.startPing()
	case r == 'p':
		a.startPing()
	case r == 'd' || r == 'D':
		a.activate(-1)
	case r == 'a':
		a.beginForm(Device{Port: 22}, false)
	case r == 'e' && hasDevice:
		a.beginForm(d, true)
	case r == 'v' && hasDevice:
		a.Mode = "details"
	case key == tcell.KeyDelete && hasDevice:
		a.Pending = &action{Kind: "delete", Device: d}
	case key == tcell.KeyEnter && hasDevice:
		for i, s := range a.Sessions {
			s.mu.Lock()
			same := s.Device.ID == d.ID && !s.Exited
			s.mu.Unlock()
			if same {
				a.activate(i + 1)
				return
			}
		}
		if a.Opening {
			return
		}
		a.Opening = true
		a.Message = "Opening SSH..."
		a.openingWork.Add(1)
		go func() {
			defer a.openingWork.Done()
			s, err := a.opener(d, max(1, w), max(1, h-2), a.notify)
			if a.ctx.Err() != nil {
				if s != nil {
					s.Close()
				}
				return
			}
			select {
			case a.messages <- openedSession{s, err}:
			case <-a.ctx.Done():
				if s != nil {
					s.Close()
				}
			}
		}()
	}
	a.Selected = max(0, a.Selected)
}
func editText(text *string, cursor *int, event *tcell.EventKey, limit int) {
	chars := []rune(*text)
	*cursor = max(0, min(*cursor, len(chars)))
	switch event.Key() {
	case tcell.KeyLeft:
		*cursor = max(0, *cursor-1)
	case tcell.KeyRight:
		*cursor = min(len(chars), *cursor+1)
	case tcell.KeyHome:
		*cursor = 0
	case tcell.KeyEnd:
		*cursor = len(chars)
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if *cursor > 0 {
			chars = append(chars[:*cursor-1], chars[*cursor:]...)
			*cursor--
		}
	case tcell.KeyDelete:
		if *cursor < len(chars) {
			chars = append(chars[:*cursor], chars[*cursor+1:]...)
		}
	case tcell.KeyCtrlU:
		chars = nil
		*cursor = 0
	case tcell.KeyRune:
		r := event.Rune()
		if len(chars) < limit && !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) && event.Modifiers()&(tcell.ModCtrl|tcell.ModAlt) == 0 {
			chars = append(chars, 0)
			copy(chars[*cursor+1:], chars[*cursor:])
			chars[*cursor] = r
			*cursor++
		}
	}
	*text = string(chars)
}
func (a *App) beginForm(d Device, edit bool) {
	a.Mode = "form"
	a.Form = formState{Values: [5]string{d.Name, d.Host, d.Username, strconv.Itoa(d.Port), d.Description}}
	if edit {
		a.Form.EditID = d.ID
	}
	a.Form.Cursor = len([]rune(d.Name))
}
func (a *App) formKey(event *tcell.EventKey) {
	f := &a.Form
	a.Message = ""
	switch event.Key() {
	case tcell.KeyTAB, tcell.KeyDown:
		f.Field = (f.Field + 1) % 5
		f.Cursor = len([]rune(f.Values[f.Field]))
	case tcell.KeyBacktab, tcell.KeyUp:
		f.Field = (f.Field + 4) % 5
		f.Cursor = len([]rune(f.Values[f.Field]))
	case tcell.KeyEnter:
		port, err := strconv.Atoi(f.Values[3])
		if err != nil {
			a.setMessage(errors.New("SSH port must be a number"), "")
			return
		}
		saved, err := a.Store.Save(Device{Name: f.Values[0], Host: f.Values[1], Username: f.Values[2], Port: port, Description: f.Values[4]}, f.EditID)
		if err != nil {
			a.setMessage(err, "")
			return
		}
		a.Mode = "list"
		a.Query = ""
		err = a.refresh()
		for i, d := range a.Filtered {
			if d.ID == saved.ID {
				a.Selected = i
			}
		}
		a.startPing()
		a.setMessage(err, "Saved "+saved.Name+".")
	default:
		editText(&f.Values[f.Field], &f.Cursor, event, []int{60, 253, 64, 5, 240}[f.Field])
	}
}
func (a *App) discoveryKey(event *tcell.EventKey) {
	s := &a.Scan
	key, r := event.Key(), event.Rune()
	if s.Editing {
		if key == tcell.KeyEscape {
			s.Editing = false
			s.Message = ""
		} else if key == tcell.KeyEnter {
			cidr, _, err := SubnetHosts(s.Input)
			if err != nil {
				s.Message = err.Error()
				return
			}
			index := -1
			for i, rangeItem := range s.Ranges {
				if rangeItem.CIDR == cidr {
					index = i
				}
			}
			if index < 0 {
				s.Ranges = append(s.Ranges, ScanRange{cidr, "Custom"})
				index = len(s.Ranges) - 1
			}
			s.RangeIndex = index
			s.Editing = false
			a.startScan()
		} else {
			editText(&s.Input, &s.Cursor, event, 20)
		}
		return
	}
	s.Message = ""
	switch {
	case key == tcell.KeyEscape:
		a.activate(0)
	case r == 'q':
		a.quit()
	case r == 'r':
		a.startScan()
	case r == 'x':
		if s.Cancel != nil {
			s.Cancel()
		}
		s.Generation++
		s.Running = false
	case r == 'c':
		s.Editing = true
		s.Input = ""
		if s.RangeIndex < len(s.Ranges) {
			s.Input = s.Ranges[s.RangeIndex].CIDR
		}
		s.Cursor = len(s.Input)
	case key == tcell.KeyLeft || key == tcell.KeyRight:
		if len(s.Ranges) > 0 {
			delta := 1
			if key == tcell.KeyLeft {
				delta = -1
			}
			s.RangeIndex = (s.RangeIndex + delta + len(s.Ranges)) % len(s.Ranges)
		}
	case key == tcell.KeyUp || r == 'k':
		s.Selected = max(0, s.Selected-1)
	case key == tcell.KeyDown || r == 'j':
		s.Selected = max(0, min(len(s.Results)-1, s.Selected+1))
	case key == tcell.KeyEnter || r == 'a':
		if s.Selected >= len(s.Results) {
			return
		}
		found := s.Results[s.Selected]
		a.activate(0)
		a.Query = ""
		a.refresh()
		for i, d := range a.Filtered {
			if d.Host == found.Host && d.Port == 22 {
				a.Selected = i
				a.Mode = "list"
				a.Message = "Selected saved device. Press Enter to connect."
				return
			}
		}
		a.beginForm(Device{Name: "Device " + found.Host, Host: found.Host, Port: 22, Description: "Discovered open port 22"}, false)
		if found.Confirmed {
			a.Form.Values[4] = "Discovered SSH device"
		}
		a.Form.Field = 2
		a.Form.Cursor = 0
	}
}
