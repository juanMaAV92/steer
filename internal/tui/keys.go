package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap centraliza los atajos de la TUI (habilita ? help y rebinds futuros).
type keyMap struct {
	Up, Down, Tab, ShiftTab, Enter, Esc          key.Binding
	Deploy, Scale, Rollback, Refresh, Help, Quit key.Binding
	Left, Right, Context                         key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k")),
		Down:     key.NewBinding(key.WithKeys("down", "j")),
		Tab:      key.NewBinding(key.WithKeys("tab")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab")),
		Enter:    key.NewBinding(key.WithKeys("enter")),
		Esc:      key.NewBinding(key.WithKeys("esc")),
		Deploy:   key.NewBinding(key.WithKeys("d")),
		Scale:    key.NewBinding(key.WithKeys("s")),
		Rollback: key.NewBinding(key.WithKeys("R")),
		Refresh:  key.NewBinding(key.WithKeys("r")),
		Help:     key.NewBinding(key.WithKeys("?")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c")),
		Left:     key.NewBinding(key.WithKeys("left", "h")),
		Right:    key.NewBinding(key.WithKeys("right", "l")),
		Context:  key.NewBinding(key.WithKeys("c")),
	}
}

func (k keyMap) shortHelp() string {
	return "↑↓/click select · tab switch panel · d deploy · s scale · R rollback · r refresh · c context · ? help · q quit"
}
