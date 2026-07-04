// internal/tui/app.go
package tui

import (
	"context"
	"errors"
	"fmt"
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

// deployState agrupa el estado del watch de deploy en vivo; Reset lo limpia entero.
type deployState struct {
	Active  bool
	Done    bool
	Service string
	LastID  string
}

func (d *deployState) Reset() { *d = deployState{} }

// Model es el estado raíz de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	runCtx   context.Context
	factory  providers.ProviderFactory
	provider providers.Provider
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

	width, height           int
	sidebarW, panelW, bodyH int
	singleColumn            bool
	deploy                  deployState
}

func New(ctx context.Context, factory providers.ProviderFactory, contexts []config.Context, current config.Context) Model {
	provider, err := factory(ctx, current)
	var dep core.Deployer
	if err == nil {
		dep, err = provider.Deployer()
	}
	m := Model{
		runCtx: ctx, factory: factory, provider: provider, contexts: contexts, current: current,
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
	ctx := m.runCtx
	dep := m.dep
	return func() tea.Msg {
		s, err := dep.ListServices(ctx)
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
			m.deploy.Done = true
			return m, m.loadServicesCmd()
		}
		m.deploy.LastID = msg.lastID
		return m, deployPollCmd(m.runCtx, m.dep, m.deploy.Service, m.deploy.LastID)

	case deployPollMsg:
		if msg.err != nil {
			m.events.AppendLine(render.Danger("error: " + msg.err.Error()))
			m.deploy.Done = true
			return m, m.loadServicesCmd()
		}
		for i := len(msg.events) - 1; i >= 0; i-- {
			e := msg.events[i]
			line := "[" + e.At.Format("15:04:05") + "] " + e.Message
			if e.IsError {
				m.events.AppendLine(render.Danger(line))
			} else {
				m.events.AppendLine(render.Dim(line))
			}
		}
		m.deploy.LastID = msg.lastID
		m.events.SetStatusLine("Rollout: " + render.Rollout(string(msg.rollout)) +
			" | Running: " + strconv.Itoa(msg.running) +
			" | Pending: " + strconv.Itoa(msg.pending) +
			" | Desired: " + strconv.Itoa(msg.desired))
		if msg.done {
			m.events.AppendLine(render.Success("✓ deployment completed"))
			m.deploy.Active = false
			m.deploy.Done = true
			return m, m.loadServicesCmd()
		}
		if msg.failed {
			m.events.AppendLine(render.Danger("✗ deployment failed"))
			m.deploy.Active = false
			m.deploy.Done = true
			return m, m.loadServicesCmd()
		}
		return m, deployTickCmd()

	case deployPollTickMsg:
		if m.deploy.Active && !m.deploy.Done {
			return m, deployPollCmd(m.runCtx, m.dep, m.deploy.Service, m.deploy.LastID)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		// mientras el picker está abierto, el overlay captura el mouse (click-to-select)
		if m.focus == focusContextPicker {
			return m.handlePickerMouse(msg)
		}
		return m, m.handleMouse(msg)
	}
	return m, nil
}

// handlePickerMouse maneja el mouse mientras el selector de contexto está abierto:
// un click izquierdo sobre una fila la conmuta; cualquier otro evento se traga
// (no cierra el overlay ni muta el estado por debajo).
func (m Model) handlePickerMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		// el picker se renderiza tras la barra superior: línea de picker = Y - topBarHeight
		if idx, ok := m.picker.indexAtLine(msg.Y - topBarHeight); ok {
			m.picker.selectIndex(idx)
			return m.applyContextSwitch()
		}
	}
	return m, nil
}

// handleMouse enruta los eventos de mouse a la zona correcta:
// rueda → scroll del panel de eventos, click izquierdo → selección en sidebar o pestaña en panel.
func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// con el modal de acción abierto, cualquier click lo cancela (captura el mouse)
	if m.focus == focusAction {
		if msg.Action == tea.MouseActionPress {
			m.action.close()
			m.focus = focusSidebar
		}
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
	// click en la barra superior (contexto) → abrir el selector de contexto
	if msg.Y < topBarHeight {
		m.picker = newContextPicker(m.contexts, m.current.Name)
		m.notice = ""
		m.focus = focusContextPicker
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
	// click en la fila de botones de acción del panel Details
	// La geometría de botones solo es válida en modo dos columnas; en una columna Y=11 cae
	// dentro del sidebar y provocaría un misfire que abre el modal de acción erróneamente.
	const detailsButtonRowY = 11 // topBar(1)+borde(1)+tabs(1)+blanco(1)+details: name,blank,4 stats,blank,botones
	if !m.singleColumn && m.current.Writable && m.tabs.Active == panel.TabDetails && msg.Y == detailsButtonRowY {
		localX := msg.X - (m.sidebarW + 3)
		if idx := render.ButtonAtColumn(panel.DetailsActionLabels, localX); idx >= 0 {
			m.openActionKind(actionKindFor(idx))
			return nil
		}
	}
	// click en la zona del panel: primera fila útil = pestañas
	panelRow := msg.Y - (topBarHeight + borderTop)
	if panelRow == 0 {
		// el contenido del panel empieza tras el borde del sidebar + el borde del panel:
		// sidebar = [borde][contenido sidebarW][borde], luego [borde panel][contenido].
		panelContentX0 := m.sidebarW + 3
		if idx := m.tabs.TabAtColumn(msg.X - panelContentX0); idx >= 0 {
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
				m.deploy = deployState{Active: true, Service: svc}
				return m, startDeployCmd(m.runCtx, m.dep, svc, tag)
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
	provider, err := m.factory(m.runCtx, sel)
	if err == nil {
		var dep core.Deployer
		dep, err = provider.Deployer()
		if err == nil {
			m.provider = provider
			m.dep = dep
		}
	}
	if err != nil {
		if errors.Is(err, providers.ErrProviderNotImplemented) {
			m.notice = "provider " + strconv.Quote(sel.Cloud) + " not implemented yet"
		} else {
			m.notice = "switch failed: " + err.Error()
		}
		m.focus = focusSidebar
		return m, nil // conserva el contexto previo
	}
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
	m.deploy.Reset()
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
	dep := m.dep
	ctx := m.runCtx
	m.action.close()
	m.focus = focusSidebar
	return func() tea.Msg {
		switch a.kind {
		case actionRollback:
			return actionDoneMsg{msg: "rolled back " + a.service, err: dep.Rollback(ctx, a.service)}
		case actionDeploy:
			// El deploy SIEMPRE va por startDeployCmd (flujo en vivo con eventos).
			// Esta rama solo es alcanzable si un refactor rompe el guard de Enter.
			return actionDoneMsg{err: fmt.Errorf("internal: deploy must go through startDeployCmd")}
		case actionScale:
			n, convErr := strconv.Atoi(a.input)
			if convErr != nil {
				return actionDoneMsg{err: convErr}
			}
			return actionDoneMsg{msg: "scaled " + a.service + " to " + a.input,
				err: dep.Scale(ctx, a.service, n)}
		}
		return actionDoneMsg{}
	}
}

// actionKindFor mapea el índice del botón de Details a su actionKind.
func actionKindFor(idx int) actionKind {
	switch idx {
	case 1:
		return actionScale
	case 2:
		return actionRollback
	default:
		return actionDeploy
	}
}

// openActionKind abre el modal para una acción (usado por el click en botones de Details).
func (m *Model) openActionKind(kind actionKind) {
	s, ok := m.sidebar.selected()
	if !ok {
		return
	}
	m.action.open(kind, s.Name)
	m.notice = ""
	m.focus = focusAction
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

	// modal de acción: retorna antes de construir el layout (no renderizar para tirar)
	if m.focus == focusAction {
		return top + "\n" + m.action.modalView(m.width, m.bodyH) + "\n" +
			bottomBar(m.keys.shortHelp(), m.notice, m.status)
	}

	sideStyle := blurredBorder()
	panelStyle := blurredBorder()
	switch m.focus {
	case focusSidebar:
		sideStyle = focusedBorder()
	case focusPanel:
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
