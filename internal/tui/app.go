// internal/tui/app.go
package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/juanMaAV92/steer/internal/tui/panel"
)

type focus int

const (
	focusSidebar focus = iota
	focusPanel
	focusAction
	focusContextPicker
)

// Constantes de geometría para el routing de mouse.
// topBarHeight: la barra superior ocupa 1 línea.
// borderTop: fila del borde superior del sidebar/panel (1 fila del borde lipgloss redondeado).
// sidebarHeader: línea "SERVICES (n)" dentro del borde.
const (
	topBarHeight  = 1 // la barra superior ocupa 1 línea
	borderTop     = 1 // borde superior del sidebar/panel
	sidebarHeader = 1 // línea "SERVICES (n)" dentro del borde
)

type actionKind int

const (
	actionRollback actionKind = iota
	actionDeploy
	actionScale
)

// Model es el estado raíz de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	factory  providers.DeployerFactory
	contexts []config.Context
	current  config.Context
	dep      core.Deployer
	depErr   error
	keys     keyMap

	sidebar sidebar
	tabs    panel.Tabs
	events  panel.Events
	action  action
	picker  contextPicker

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

func New(factory providers.DeployerFactory, contexts []config.Context, current config.Context) Model {
	dep, err := factory(current)
	m := Model{
		factory: factory, contexts: contexts, current: current,
		dep: dep, depErr: err,
		keys: defaultKeys(), sidebar: newSidebar(), events: panel.NewEvents(),
		loading: err == nil,
	}
	m.sidebar.prefix = current.Prefix()
	if err != nil {
		m.err = err
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.depErr != nil {
		return nil
	}
	return tea.Batch(m.loadServicesCmd(), tickCmd())
}

func (m Model) loadServicesCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := m.dep.ListServices(context.Background(), m.current.Cluster)
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
		m.sidebar.width = m.sidebarW
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
	m.sidebar.width = m.sidebarW
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

	case deployStartedMsg:
		for _, s := range msg.steps {
			m.events.AppendLine(render.Dim("[*] " + s))
		}
		if msg.err != nil {
			m.events.AppendLine(render.Danger("error: " + msg.err.Error()))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		m.deployLastID = msg.lastID
		return m, deployPollCmd(m.dep, m.current.Cluster, m.deployService, m.deployLastID)

	case deployPollMsg:
		if msg.err != nil {
			m.events.AppendLine(render.Danger("error: " + msg.err.Error()))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		for i := len(msg.events) - 1; i >= 0; i-- {
			e := msg.events[i]
			m.events.AppendLine(render.Dim("[" + e.At.Format("15:04:05") + "] " + e.Message))
		}
		m.deployLastID = msg.lastID
		m.events.SetStatusLine("Rollout: " + rolloutColored(msg.rollout) +
			" | Running: " + strconv.Itoa(msg.running) +
			" | Pending: " + strconv.Itoa(msg.pending) +
			" | Desired: " + strconv.Itoa(msg.desired))
		if msg.done {
			m.events.AppendLine(render.Success("✓ deployment completed"))
			m.deployActive = false
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		if msg.failed {
			m.events.AppendLine(render.Danger("✗ deployment failed"))
			m.deployActive = false
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		return m, deployTickCmd()

	case deployPollTickMsg:
		if m.deployActive && !m.deployDone {
			return m, deployPollCmd(m.dep, m.current.Cluster, m.deployService, m.deployLastID)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m, m.handleMouse(msg)
	}
	return m, nil
}

// handleMouse enruta los eventos de mouse a la zona correcta:
// rueda → scroll del panel de eventos, click izquierdo → selección en sidebar o pestaña en panel.
func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// mientras el overlay del picker esté abierto, capturar todos los eventos de mouse
	// para evitar que caigan al sidebar/panel y muten el estado por debajo
	if m.focus == focusContextPicker {
		return nil
	}
	// rueda: scroll en el panel de eventos si el cursor está sobre el panel
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		if msg.X > m.sidebarW {
			return m.events.Update(msg)
		}
		return nil
	}
	// solo procesar clicks izquierdos
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	// click en la zona del sidebar
	if msg.X <= m.sidebarW {
		row := msg.Y - (topBarHeight + borderTop + sidebarHeader)
		if row >= 0 && row < m.sidebar.serviceRowCount() {
			m.sidebar.selectIndex(row)
			m.focus = focusSidebar
		}
		return nil
	}
	// click en la zona del panel: primera fila útil = pestañas
	panelRow := msg.Y - (topBarHeight + borderTop)
	if panelRow == 0 {
		seg := m.panelW / m.tabs.Count()
		if seg < 1 {
			seg = 1
		}
		idx := (msg.X - m.sidebarW) / seg
		if idx >= 0 && idx < m.tabs.Count() {
			m.tabs.Set(panel.Tab(idx))
		}
	}
	m.focus = focusPanel
	return nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// foco en overlay del selector de contexto: captura el input antes del switch global
	if m.focus == focusContextPicker {
		switch {
		case msg.Type == tea.KeyCtrlC:
			return m, tea.Quit
		case key.Matches(msg, m.keys.Esc):
			m.focus = focusSidebar
			return m, nil
		case key.Matches(msg, m.keys.Enter):
			return m.applyContextSwitch()
		case key.Matches(msg, m.keys.Down):
			m.picker.moveDown()
		case key.Matches(msg, m.keys.Up):
			m.picker.moveUp()
		}
		return m, nil
	}

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
			if m.action.kind == actionDeploy {
				// el flujo de deploy en vivo se arranca directamente, no via runActionCmd
				svc, tag := m.action.service, m.action.input
				m.action.close()
				m.focus = focusPanel
				m.tabs.Active = panel.TabEvents
				m.events.Reset()
				m.deployActive, m.deployDone = true, false
				m.deployService = svc
				return m, startDeployCmd(m.dep, m.current.Cluster, svc, tag)
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
		m.status = ""
		m.notice = ""
		return m, m.loadServicesCmd()
	case msg.String() == "c":
		// abre el overlay de selección de contexto
		m.picker = newContextPicker(m.contexts, m.current.Name)
		m.notice = ""
		m.focus = focusContextPicker
		return m, nil
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
		// cambiar pestaña: derecha/izquierda (cíclico en ambos sentidos)
		switch msg.String() {
		case "right", "l":
			m.tabs.Next()
		case "left", "h":
			m.tabs.Prev()
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

// applyContextSwitch conmuta al contexto seleccionado en el picker.
// Si el provider no está implementado o la fábrica falla, muestra un notice y no cambia.
func (m Model) applyContextSwitch() (tea.Model, tea.Cmd) {
	sel, ok := m.picker.selected()
	if !ok {
		m.focus = focusSidebar
		return m, nil
	}
	if sel.Name == m.current.Name {
		m.focus = focusSidebar
		return m, nil
	}
	dep, err := m.factory(sel)
	if err != nil {
		if errors.Is(err, providers.ErrProviderNotImplemented) {
			m.notice = "provider " + strconv.Quote(sel.Cloud) + " not implemented yet"
		} else {
			m.notice = "switch failed: " + err.Error()
		}
		m.focus = focusSidebar
		return m, nil // conserva el contexto previo
	}
	m.dep = dep
	m.current = sel
	m.sidebar = newSidebar()
	m.sidebar.prefix = sel.Prefix()
	m.sidebar.width = m.sidebarW
	m.loading = true
	m.notice = ""
	m.status = ""
	m.focus = focusSidebar
	// reiniciar el estado de watch de deploy para evitar que el loop de poll
	// siga disparándose contra el nuevo deployer con el servicio anterior
	m.deployActive = false
	m.deployDone = false
	m.deployService = ""
	m.deployLastID = ""
	m.events.Reset()
	m.tabs.Active = panel.TabDetails
	return m, m.loadServicesCmd()
}

func (m Model) openAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.current.Writable {
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
	dep, cluster := m.dep, m.current.Cluster
	m.action.close()
	m.focus = focusSidebar
	return func() tea.Msg {
		ctx := context.Background()
		switch a.kind {
		case actionRollback:
			return actionDoneMsg{msg: "rolled back " + a.service, err: dep.Rollback(ctx, cluster, a.service)}
		case actionDeploy:
			// inalcanzable: el deploy se inicia desde el handler de Enter (startDeployCmd), no aquí
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
	top := topBar(m.current.Cloud, m.current.Name, m.current.Cluster, m.current.Writable)

	// overlay del selector de contexto: reemplaza el cuerpo principal
	if m.focus == focusContextPicker {
		return top + "\n" + m.picker.view() + "\n" + bottomBar(m.keys.shortHelp(), m.notice, m.status)
	}

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
		displayName := strings.TrimPrefix(s.Name, m.current.Prefix())
		return panel.DetailsView(s, m.current.Writable, displayName)
	}
}

// rolloutColored colorea el estado del rollout según su nivel.
func rolloutColored(state string) string {
	switch state {
	case "COMPLETED":
		return render.Success(state)
	case "FAILED":
		return render.Danger(state)
	default:
		return render.Accent(state)
	}
}
