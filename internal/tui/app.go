// internal/tui/app.go
package tui

import (
	"context"
	"strconv"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/juanMaAV92/steer/internal/tui/panel"
)

type focus int

const (
	focusSidebar focus = iota
	focusPanel
	focusAction
)

type actionKind int

const (
	actionRollback actionKind = iota
	actionDeploy
	actionScale
)

// Model es el estado raíz de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	dep      core.Deployer
	cluster  string
	env      string
	writable bool
	keys     keyMap

	sidebar sidebar
	tabs    panel.Tabs
	events  panel.Events
	action  action

	focus   focus
	loading bool
	notice  string
	status  string
	err     error

	width, height            int
	sidebarW, panelW, bodyH  int
	singleColumn             bool
	deployActive, deployDone bool
	deployLastID             string
	deployService            string
}

func New(dep core.Deployer, cluster, env string, writable bool) Model {
	return Model{
		dep: dep, cluster: cluster, env: env, writable: writable,
		keys: defaultKeys(), sidebar: newSidebar(), events: panel.NewEvents(),
		loading: true,
	}
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.loadServicesCmd(), tickCmd()) }

func (m Model) loadServicesCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := m.dep.ListServices(context.Background(), m.cluster)
		return servicesMsg{services: s, err: err}
	}
}

// layout reparte el espacio disponible entre sidebar y panel.
// Si el ancho < singleColumnThreshold, colapsa a una sola columna apilada.
func (m *Model) layout() {
	m.singleColumn = m.width < singleColumnThreshold
	m.bodyH = m.height - 4 // top bar (3) + bottom (1)
	if m.bodyH < 3 {
		m.bodyH = 3
	}
	if m.singleColumn {
		m.sidebarW = m.width - 4
		m.panelW = m.width - 4
		if m.sidebarW < 10 {
			m.sidebarW, m.panelW = 10, 10
		}
		m.events.SetSize(m.panelW-2, m.bodyH/2-3)
		return
	}
	m.sidebarW = m.width * 30 / 100
	if m.sidebarW < sidebarMinWidth {
		m.sidebarW = sidebarMinWidth
	}
	m.panelW = m.width - m.sidebarW - 4 // bordes
	if m.panelW < 10 {
		m.panelW = 10
	}
	m.events.SetSize(m.panelW-2, m.bodyH-3) // -tabs -bordes
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case servicesMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.sidebar.setServices(msg.services)
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadServicesCmd(), tickCmd())

	case actionDoneMsg:
		m.focus = focusSidebar
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
	// foco en overlay de acción: captura el input
	if m.focus == focusAction {
		switch {
		case key.Matches(msg, m.keys.Quit) && msg.Type == tea.KeyCtrlC:
			return m, tea.Quit
		case key.Matches(msg, m.keys.Esc):
			m.action.close()
			m.focus = focusSidebar
			return m, nil
		case key.Matches(msg, m.keys.Enter):
			if !m.action.ready() {
				return m, nil
			}
			return m, m.runActionCmd()
		default:
			m.action.typeKey(msg)
			return m, nil
		}
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.ShiftTab):
		if m.focus == focusSidebar {
			m.focus = focusPanel
		} else {
			m.focus = focusSidebar
		}
		return m, nil
	case key.Matches(msg, m.keys.Refresh):
		m.loading = true
		return m, m.loadServicesCmd()
	case key.Matches(msg, m.keys.Deploy), key.Matches(msg, m.keys.Scale), key.Matches(msg, m.keys.Rollback):
		return m.openAction(msg)
	}

	if m.focus == focusPanel {
		switch {
		case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.Up):
			// m.events es receptor-valor, pero la copia mutada se preserva porque se retorna via `return m, cmd`.
			cmd := m.events.Update(msg)
			return m, cmd
		}
		// permitir cambiar pestaña con left/right
		switch msg.String() {
		case "right", "l":
			m.tabs.Next()
		}
		return m, nil
	}

	// foco en sidebar
	switch {
	case key.Matches(msg, m.keys.Down):
		m.sidebar.moveDown()
	case key.Matches(msg, m.keys.Up):
		m.sidebar.moveUp()
	}
	return m, nil
}

func (m Model) openAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.writable {
		m.notice = "read-only environment (writable=false) — action blocked"
		return m, nil
	}
	s, ok := m.sidebar.selected()
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Deploy):
		m.action.open(actionDeploy, s.Name)
	case key.Matches(msg, m.keys.Scale):
		m.action.open(actionScale, s.Name)
	case key.Matches(msg, m.keys.Rollback):
		m.action.open(actionRollback, s.Name)
	}
	m.notice = ""
	m.focus = focusAction
	return m, nil
}

func (m *Model) runActionCmd() tea.Cmd {
	a := m.action
	dep, cluster := m.dep, m.cluster
	m.action.close()
	m.focus = focusSidebar
	return func() tea.Msg {
		ctx := context.Background()
		switch a.kind {
		case actionRollback:
			return actionDoneMsg{msg: "rolled back " + a.service, err: dep.Rollback(ctx, cluster, a.service)}
		case actionDeploy:
			return actionDoneMsg{msg: "deployed " + a.service + " -> " + a.input,
				err: dep.Deploy(ctx, cluster, a.service, a.input, nil)}
		case actionScale:
			n, convErr := strconv.Atoi(a.input)
			if convErr != nil {
				return actionDoneMsg{err: convErr}
			}
			return actionDoneMsg{msg: "scaled " + a.service + " to " + a.input,
				err: dep.Scale(ctx, cluster, a.service, n)}
		}
		return actionDoneMsg{}
	}
}

func (m Model) View() string {
	if m.err != nil {
		return render.Danger("error: "+m.err.Error()) + "\n" + render.Dim("press q to quit")
	}
	top := topBar("aws", m.env, m.cluster, m.writable)

	sideStyle := blurredBorder()
	panelStyle := blurredBorder()
	if m.focus == focusSidebar {
		sideStyle = focusedBorder()
	} else if m.focus == focusPanel {
		panelStyle = focusedBorder()
	}
	panelBody := m.tabs.View() + "\n\n" + m.panelBody()
	var body string
	if m.singleColumn {
		// apilado: sidebar arriba, panel abajo (cada uno la mitad del alto)
		side := sideStyle.Width(m.sidebarW).Height(m.bodyH / 2).Render(m.sidebar.view())
		pan := panelStyle.Width(m.panelW).Height(m.bodyH / 2).Render(panelBody)
		body = lipgloss.JoinVertical(lipgloss.Left, side, pan)
	} else {
		side := sideStyle.Width(m.sidebarW).Height(m.bodyH).Render(m.sidebar.view())
		pan := panelStyle.Width(m.panelW).Height(m.bodyH).Render(panelBody)
		body = lipgloss.JoinHorizontal(lipgloss.Top, side, pan)
	}

	bottom := bottomBar(m.keys.shortHelp(), m.notice, m.status)
	if m.focus == focusAction {
		bottom = m.action.view()
	}
	return top + "\n" + body + "\n" + bottom
}

func (m Model) panelBody() string {
	s, ok := m.sidebar.selected()
	if !ok {
		return render.Dim("no service selected")
	}
	switch m.tabs.Active {
	case panel.TabEvents:
		return m.events.View()
	case panel.TabLogs:
		return panel.LogsView()
	default:
		return panel.DetailsView(s, m.writable)
	}
}
