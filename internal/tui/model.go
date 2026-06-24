// Package tui implementa el dashboard interactivo (steer tui) sobre core.Deployer.
package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

type viewState int

const (
	viewList viewState = iota
	viewDetail
)

// Model es el estado de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	dep      core.Deployer
	cluster  string
	env      string
	services []core.ServiceStatus
	cursor   int
	view     viewState
	loading  bool
	status   string
	err      error
}

// servicesMsg transporta el resultado de listar servicios.
type servicesMsg struct {
	services []core.ServiceStatus
	err      error
}

// New crea el modelo inicial.
func New(dep core.Deployer, cluster, env string) Model {
	return Model{dep: dep, cluster: cluster, env: env, loading: true}
}

// loadServicesCmd lista los servicios en segundo plano.
func (m Model) loadServicesCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := m.dep.ListServices(context.Background(), m.cluster)
		return servicesMsg{services: s, err: err}
	}
}

func (m Model) Init() tea.Cmd {
	return m.loadServicesCmd()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case servicesMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.services = msg.services
		if m.cursor >= len(m.services) {
			m.cursor = max(0, len(m.services)-1)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.services)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	}
	return m, nil
}

// View implements tea.Model (placeholder).
func (m Model) View() string {
	return ""
}
