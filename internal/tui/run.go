package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

// Run abre la TUI a pantalla completa con soporte de mouse hasta que el usuario sale.
func Run(dep core.Deployer, cluster, env string, writable bool) error {
	p := tea.NewProgram(
		New(dep, cluster, env, writable),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
