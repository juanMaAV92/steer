package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/providers"
)

// Run abre la TUI a pantalla completa con soporte de mouse hasta que el usuario sale.
func Run(factory providers.DeployerFactory, contexts []config.Context, current config.Context) error {
	p := tea.NewProgram(
		New(factory, contexts, current),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
