// Package tui implementa el dashboard interactivo (steer tui) sobre core.Deployer.
package tui

import (
	"context"
	"time"

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

// tickMsg dispara un auto-refresh periódico.
type tickMsg struct{}

const refreshInterval = 15 * time.Second

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
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
	return tea.Batch(m.loadServicesCmd(), tickCmd())
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

	case tickMsg:
		return m, tea.Batch(m.loadServicesCmd(), tickCmd())

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	if m.view == viewDetail {
		if msg.String() == "esc" {
			m.view = viewList
		}
		return m, nil
	}

	// viewList
	switch msg.String() {
	case "r":
		m.loading = true
		return m, m.loadServicesCmd()
	case "j", "down":
		if m.cursor < len(m.services)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "enter":
		if len(m.services) > 0 {
			m.view = viewDetail
		}
	}
	return m, nil
}

// selected devuelve el servicio bajo el cursor (ok=false si la lista está vacía).
func (m Model) selected() (core.ServiceStatus, bool) {
	if m.cursor < 0 || m.cursor >= len(m.services) {
		return core.ServiceStatus{}, false
	}
	return m.services[m.cursor], true
}

// View implements tea.Model (placeholder).
func (m Model) View() string {
	return ""
}
