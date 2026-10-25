package main

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

type frameMsg struct{}

func frame() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return frameMsg{} })
}

type model struct {
	world         *World
	id            int
	width, height int

	eventTypes     bool
	heldL, heldR   bool
	untilL, untilR time.Time
	lastInput      time.Time
	quickChat      bool

	cache    string
	cacheVer uint64
	dirty    bool
}

func newModel(w *World, id, width, height int) *model {
	return &model{world: w, id: id, width: width, height: height, lastInput: time.Now(), dirty: true}
}

func (m *model) Init() tea.Cmd { return frame() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.dirty = msg.Width, msg.Height, true
	case tea.KeyboardEnhancementsMsg:
		m.eventTypes = msg.SupportsEventTypes()
	case tea.BlurMsg:
		m.release()
	case tea.KeyPressMsg:
		m.lastInput = time.Now()
		return m, m.key(msg.Key())
	case tea.KeyReleaseMsg:
		if k := msg.Key(); isLeft(k) {
			m.heldL = false
		} else if isRight(k) {
			m.heldR = false
		}
		m.push()
	case frameMsg:
		if time.Since(m.lastInput) > 10*time.Minute {
			return m, tea.Quit
		}
		m.push()
		return m, frame()
	}
	return m, nil
}

// input processing
func (m *model) key(k tea.Key) tea.Cmd {
	if m.quickChat {
		switch {
		case k.Code >= '1' && k.Code <= '4':
			m.world.Say(m.id, int(k.Code-'1'))
			m.quickChat, m.dirty = false, true
			return nil
		case k.Code == 'q', k.Code == tea.KeyEscape:
			m.quickChat, m.dirty = false, true
			return nil
		}
	}

	now := time.Now()
	hold := func(until time.Time) time.Time {
		if now.Before(until) {
			return now.Add(120 * time.Millisecond)
		}
		return now.Add(400 * time.Millisecond)
	}
	switch {
	case k.Code == 'c' && k.Mod == tea.ModCtrl, k.Code == 'q', k.Code == tea.KeyEscape:
		return tea.Quit
	case k.Code == 't', k.Code == tea.KeyEnter:
		m.quickChat, m.dirty = true, true
	case isLeft(k):
		m.heldL, m.heldR, m.untilL, m.untilR = true, false, hold(m.untilL), time.Time{}
	case isRight(k):
		m.heldR, m.heldL, m.untilR, m.untilL = true, false, hold(m.untilR), time.Time{}
	case k.Code == tea.KeyDown, k.Code == 's':
		m.release()
	case k.Code == tea.KeySpace, k.Code == tea.KeyUp, k.Code == 'w', k.Code == 'k':
		if !k.IsRepeat {
			m.world.Jump(m.id)
		}
	}
	m.push()
	return nil
}

// stops all movement when not focused
func (m *model) release() {
	m.heldL, m.heldR, m.untilL, m.untilR = false, false, time.Time{}, time.Time{}
	m.push()
}

// sends held keys to the world, uses real key releases when the terminal supports them, otherwise falls back to hold timers
func (m *model) push() {
	if m.eventTypes {
		m.world.SetInput(m.id, m.heldL, m.heldR)
		return
	}
	now := time.Now()
	m.world.SetInput(m.id, now.Before(m.untilL), now.Before(m.untilR))
}

func (m *model) View() tea.View {
	if ver := m.world.Version(); m.dirty || ver != m.cacheVer {
		footer := "arrows/wasd move  space/w jump  t quick chat  q quit"
		if m.quickChat {
			footer = " 1 hi!  2 bye  3 oops  4 gg  q back"
		}
		m.cache, m.cacheVer, m.dirty = m.world.Render(m.id, m.width, m.height, footer), ver, false
	}
	v := tea.NewView(m.cache)
	v.AltScreen = true
	v.ReportFocus = true
	v.KeyboardEnhancements.ReportEventTypes = true
	v.WindowTitle = "SecureFormer"
	return v
}

func isLeft(k tea.Key) bool  { return k.Code == tea.KeyLeft || k.Code == 'a' || k.Code == 'h' }
func isRight(k tea.Key) bool { return k.Code == tea.KeyRight || k.Code == 'd' || k.Code == 'l' }
