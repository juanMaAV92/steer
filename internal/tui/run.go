package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

// Run abre la TUI a pantalla completa hasta que el usuario sale.
func Run(dep core.Deployer, cluster, env string) error {
	p := tea.NewProgram(New(dep, cluster, env), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
