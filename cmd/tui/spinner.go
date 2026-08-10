package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

type Spinner struct {
	frame   int
	running bool
}

var spinnerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("green"))

func NewSpinner() Spinner {
	return Spinner{
		frame:   0,
		running: false,
	}
}

func (s *Spinner) Start() {
	s.running = true
}

func (s *Spinner) Stop() {
	s.running = false
}

func (s *Spinner) View() string {
	if !s.running {
		return ""
	}

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", "⠐", "⠣", "⠩", "⠁", "⠭", "⠾", "⠷", "⠯", "⠿"}
	frame := frames[s.frame%len(frames)]
	s.frame++
	return spinnerStyle.Render(runewidth.FillRight(frame, 2))
}