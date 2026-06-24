// Package tui implementa el dashboard interactivo (steer tui) sobre core.Deployer.
package tui

import (
	"context"
	"fmt"
	"strconv"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

type viewState int

const (
	viewList viewState = iota
	viewDetail
	viewConfirm
)

type actionKind int

const (
	actionRollback actionKind = iota
	actionDeploy
	actionScale
)

type pendingAction struct {
	kind    actionKind
	service string
	input   string // tag (deploy) o count (scale)
}

// actionDoneMsg es el resultado de ejecutar una acción.
type actionDoneMsg struct {
	msg string
	err error
}

// Model es el estado de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	dep      core.Deployer
	cluster  string
	env      string
	services []core.ServiceStatus
	cursor   int
	view     viewState
	action   pendingAction
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

	case actionDoneMsg:
		m.view = viewList
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.status = msg.msg
		}
		return m, m.loadServicesCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// (1) confirm-view block: captures all runes before global q/ctrl+c
	if m.view == viewConfirm {
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEsc:
			m.view = viewList
			return m, nil
		case tea.KeyEnter:
			if m.action.kind != actionRollback && m.action.input == "" {
				return m, nil // exige input para deploy/scale
			}
			return m, m.runActionCmd()
		case tea.KeyBackspace:
			if n := len(m.action.input); n > 0 {
				m.action.input = m.action.input[:n-1]
			}
		case tea.KeyRunes:
			if m.action.kind != actionRollback {
				m.action.input += string(msg.Runes)
			}
		}
		return m, nil
	}

	// (2) global q/ctrl+c quit
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	}

	// (3) viewDetail block
	if m.view == viewDetail {
		if msg.String() == "esc" {
			m.view = viewList
		}
		return m, nil
	}

	// (4) viewList switch
	switch msg.String() {
	case "r":
		m.loading = true
		return m, m.loadServicesCmd()
	case "R":
		if s, ok := m.selected(); ok {
			m.action = pendingAction{kind: actionRollback, service: s.Name}
			m.view = viewConfirm
		}
	case "d":
		if s, ok := m.selected(); ok {
			m.action = pendingAction{kind: actionDeploy, service: s.Name}
			m.view = viewConfirm
		}
	case "s":
		if s, ok := m.selected(); ok {
			m.action = pendingAction{kind: actionScale, service: s.Name}
			m.view = viewConfirm
		}
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

func (m Model) runActionCmd() tea.Cmd {
	a := m.action
	dep := m.dep
	cluster := m.cluster
	return func() tea.Msg {
		ctx := context.Background()
		switch a.kind {
		case actionRollback:
			err := dep.Rollback(ctx, cluster, a.service)
			return actionDoneMsg{msg: "rolled back " + a.service, err: err}
		case actionDeploy:
			err := dep.Deploy(ctx, cluster, a.service, a.input, nil)
			return actionDoneMsg{msg: "deployed " + a.service + " -> " + a.input, err: err}
		case actionScale:
			n, convErr := strconv.Atoi(a.input)
			if convErr != nil {
				return actionDoneMsg{err: fmt.Errorf("invalid count %q", a.input)}
			}
			err := dep.Scale(ctx, cluster, a.service, n)
			return actionDoneMsg{msg: fmt.Sprintf("scaled %s to %d", a.service, n), err: err}
		}
		return actionDoneMsg{err: nil}
	}
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
