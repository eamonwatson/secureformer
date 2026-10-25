package main

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type splashDoneMsg struct{}

type splash struct {
	text string
	game *model
}

func (s *splash) Init() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg { return splashDoneMsg{} })
}

func (s *splash) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case splashDoneMsg:
		return s.game, s.game.Init()
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return s, tea.Quit
		}
	case tea.KeyReleaseMsg:
	default:
		_, cmd := s.game.Update(msg)
		return s, cmd
	}
	return s, nil
}

func (s *splash) View() tea.View {
	top := s.game.height / 2
	left := max(0, (s.game.width-len(s.text))/2)
	v := tea.NewView(strings.Repeat("\n", top) + strings.Repeat(" ", left) + s.text)
	v.AltScreen = true
	v.KeyboardEnhancements.ReportEventTypes = true
	return v
}
