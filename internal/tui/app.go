// internal/tui/app.go
package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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
)

// Constantes de geometría para el routing de mouse.
// topBarHeight: la barra superior ocupa 1 línea.
// borderTop: fila de la regla horizontal que separa la barra superior del cuerpo.
const (
	topBarHeight = 1 // la barra superior ocupa 1 línea
	borderTop    = 1 // fila de la regla horizontal
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
	overlay overlay
	form    *actionForm

	focus   focus
	loading bool
	notice  string
	status  string
	err     error

	width, height           int
	sidebarW, panelW, bodyH int
	singleColumn            bool
	deploy                  deployState

	tagsRepo    string // repo cuyo listado de tags está cargado/cargando
	tags        []core.ImageTag
	tagsLoading bool
	tagsErr     string
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
	m.sidebar.repoPrefix = current.RepoPrefix()
	if err == nil && current.Images != nil {
		m.sidebar.imagesState = imagesLoading
	}
	if err != nil {
		m.err = err
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.depErr != nil {
		return nil
	}
	return tea.Batch(m.loadServicesCmd(), m.loadReposCmd(), tickCmd())
}

func (m Model) loadServicesCmd() tea.Cmd {
	ctx := m.runCtx
	dep := m.dep
	return func() tea.Msg {
		s, err := dep.ListServices(ctx)
		return servicesMsg{services: s, err: err}
	}
}

// loadReposCmd pide los repos del registry; los repos no se refrescan por tick
// (cambian poco): solo al entrar al contexto y con Refresh.
func (m Model) loadReposCmd() tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		reg, err := provider.Registry()
		if errors.Is(err, core.ErrNoImagesConfig) {
			return reposMsg{disabled: true}
		}
		if err != nil {
			return reposMsg{err: err}
		}
		repos, err := reg.ListRepositories(ctx)
		return reposMsg{repos: repos, err: err}
	}
}

// loadTagsCmd pide los tags de un repo (nombre real).
func (m Model) loadTagsCmd(repo string) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		reg, err := provider.Registry()
		if err != nil {
			return tagsMsg{repo: repo, err: err}
		}
		tags, err := reg.ListTags(ctx, repo)
		return tagsMsg{repo: repo, tags: tags, err: err}
	}
}

// loadFormTagsCmd pide los tags del repo hermano del servicio para el picker.
// Cualquier error (sin config, cloud caído) degrada en silencio a input libre.
func (m Model) loadFormTagsCmd(service string) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	short := strings.TrimPrefix(service, m.current.Prefix())
	repo := m.current.RepoName(short)
	return func() tea.Msg {
		reg, err := provider.Registry()
		if err != nil {
			return formTagsMsg{service: service}
		}
		tags, err := reg.ListTags(ctx, repo)
		if err != nil {
			return formTagsMsg{service: service}
		}
		return formTagsMsg{service: service, tags: tags}
	}
}

// syncRepoTags dispara la carga de tags si la selección de repo cambió.
// Llamar tras cualquier mutación del sidebar (teclas y clicks).
func (m *Model) syncRepoTags() tea.Cmd {
	repo, ok := m.sidebar.selectedRepo()
	if !ok || repo == m.tagsRepo {
		return nil
	}
	m.tagsRepo = repo
	m.tags = nil
	m.tagsErr = ""
	m.tagsLoading = true
	return m.loadTagsCmd(repo)
}

// layout reparte el espacio: [top bar][regla][cuerpo bodyH][regla][help].
// Si el ancho < singleColumnThreshold, colapsa a una sola columna apilada.
func (m *Model) layout() {
	m.singleColumn = m.width < singleColumnThreshold
	m.bodyH = m.height - 4 // top bar + regla + regla + help
	if m.bodyH < 3 {
		m.bodyH = 3
	}
	if m.singleColumn {
		m.sidebarW = m.width
		m.panelW = m.width
		if m.sidebarW < 10 {
			m.sidebarW, m.panelW = 10, 10
		}
		m.sidebar.width = m.sidebarW - 1 // PaddingLeft(1) del bloque
		m.sidebar.height = m.bodyH / 2
		m.events.SetSize(m.panelW-2, m.bodyH/2-2)
		return
	}
	m.sidebarW = m.width * 30 / 100
	if m.sidebarW < sidebarMinWidth {
		m.sidebarW = sidebarMinWidth
	}
	m.panelW = m.width - m.sidebarW - 1 // columna del divisor
	if m.panelW < 10 {
		m.panelW = 10
	}
	m.sidebar.width = m.sidebarW - 1
	m.sidebar.height = m.bodyH
	m.events.SetSize(m.panelW-2, m.bodyH-2) // - pestañas - línea en blanco
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

	case reposMsg:
		switch {
		case msg.disabled:
			m.sidebar.imagesState = imagesDisabled
		case msg.err != nil:
			m.sidebar.imagesState = imagesError
			m.sidebar.imagesErr = msg.err.Error()
		default:
			m.sidebar.setRepos(msg.repos)
		}
		return m, nil

	case tagsMsg:
		if msg.repo != m.tagsRepo {
			return m, nil // respuesta obsoleta de un repo ya deseleccionado
		}
		m.tagsLoading = false
		if msg.err != nil {
			m.tagsErr = msg.err.Error()
			return m, nil
		}
		m.tags = msg.tags
		return m, nil

	case formTagsMsg:
		if m.form != nil && m.form.kind == actionDeploy && m.form.service == msg.service {
			m.form.setTags(msg.tags)
		}
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
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.overlay != nil {
			return m.routeOverlay(msg)
		}
		if m.form != nil {
			return m.handleFormKey(msg)
		}
		if m.sidebar.filterActive {
			return m.handleFilterKey(msg)
		}
		return m.handleKey(msg)

	case tea.MouseMsg:
		if m.overlay != nil {
			return m.routeOverlay(msg)
		}
		return m, m.handleMouse(msg)
	}
	return m, nil
}

// routeOverlay entrega el evento al overlay activo y ejecuta su resultado.
func (m Model) routeOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	done, result := m.overlay.Update(msg)
	if done {
		m.overlay = nil
		m.focus = focusSidebar
	}
	if result != nil {
		return m.handleOverlayResult(result)
	}
	return m, nil
}

// handleOverlayResult ejecuta la elección hecha en un overlay o en el formulario
// inline. NOTA: el overlay/form ya fue cerrado por el caller antes de llegar aquí.
func (m Model) handleOverlayResult(res tea.Msg) (tea.Model, tea.Cmd) {
	switch r := res.(type) {
	case contextChosenMsg:
		return m.applyContextSwitch(r.ctx)
	case actionConfirmedMsg:
		cmd := m.applyActionConfirmed(r)
		return m, cmd
	}
	return m, nil
}

// applyActionConfirmed ejecuta una acción confirmada (desde teclado o click).
// El deploy va por el flujo en vivo (Events + poll); scale/rollback por runActionCmd.
func (m *Model) applyActionConfirmed(r actionConfirmedMsg) tea.Cmd {
	if r.kind == actionDeploy {
		m.focus = focusPanel
		m.tabs.Active = panel.TabEvents
		m.events.Reset()
		m.deploy = deployState{Active: true, Service: r.service}
		return startDeployCmd(m.runCtx, m.dep, r.service, r.input)
	}
	return m.runActionCmd(r.kind, r.service, r.input)
}

// handleFormKey captura el teclado mientras el formulario de acción está abierto:
// esc cancela, enter activa el botón enfocado, tab/←/→ mueven el foco y el resto
// se teclea en el input. Las teclas globales NO disparan (modo captura).
func (m Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Esc):
		m.form = nil
	case key.Matches(msg, m.keys.Enter):
		done, result := m.form.activate()
		if done {
			m.form = nil
		}
		if result != nil {
			return m.handleOverlayResult(result)
		}
	case msg.Type == tea.KeyDown:
		m.form.movePick(1)
	case msg.Type == tea.KeyUp:
		m.form.movePick(-1)
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.Right):
		m.form.moveFocus(1)
	case key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.Left):
		m.form.moveFocus(-1)
	default:
		m.form.typeKey(msg)
	}
	return m, nil
}

// handleMouse enruta los eventos de mouse a la zona correcta:
// rueda → scroll del panel de eventos, click izquierdo → selección en sidebar o pestaña en panel.
func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// rueda: scroll del sidebar si el cursor está sobre él, del panel de eventos si no.
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -3
		}
		if msg.X < m.sidebarW {
			m.sidebar.scrollBy(delta)
			return nil
		}
		return m.events.Update(msg)
	}
	// solo procesar clicks izquierdos
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	// con el formulario abierto, el mouse solo interactúa con sus botones:
	// click fuera = no-op (solo esc o Cancel cierran)
	if m.form != nil {
		return m.clickForm(msg)
	}
	// click en la barra superior (contexto) → abrir el selector de contexto
	if msg.Y < topBarHeight {
		m.overlay = newPickerOverlay(m.keys, m.contexts, m.current.Name)
		m.notice = ""
		return nil
	}
	// click en la zona del sidebar
	if msg.X < m.sidebarW {
		row := msg.Y - (topBarHeight + borderTop)
		if e, ok := m.sidebar.EntryAtVisibleRow(row); ok {
			switch e.Kind {
			case entryHeader:
				m.sidebar.toggle(e.Section)
			case entryService, entryRepo:
				m.sidebar.selectEntry(e)
			}
			m.focus = focusSidebar
			return m.syncRepoTags()
		}
		return nil
	}
	// click en la fila de botones de acción del panel Details
	// La geometría de botones solo es válida en modo dos columnas; en una columna cae
	// dentro del sidebar y provocaría un misfire que abre el modal de acción erróneamente.
	// fila de botones de Details en pantalla, derivada del layout real del panel
	detailsButtonRowY := topBarHeight + borderTop + 1 /*pestañas*/ + 1 /*línea en blanco*/ + panel.DetailsButtonLine
	if !m.singleColumn && m.current.Writable && m.tabs.Active == panel.TabDetails && msg.Y == detailsButtonRowY {
		localX := msg.X - (m.sidebarW + 2)
		if idx := render.ButtonAtColumn(panel.DetailsActionLabels, localX); idx >= 0 {
			return m.openActionKind(actionKindFor(idx))
		}
	}
	// click en la zona del panel: primera fila útil = pestañas
	panelRow := msg.Y - (topBarHeight + borderTop)
	if panelRow == 0 {
		// el contenido del panel empieza tras el sidebar y la columna del divisor:
		// sidebar = [contenido sidebarW], luego [divisor 1col][contenido panel].
		panelContentX0 := m.sidebarW + 2
		if idx := m.tabs.TabAtColumn(msg.X - panelContentX0); idx >= 0 {
			m.tabs.Set(panel.Tab(idx))
		}
	}
	m.focus = focusPanel
	return nil
}

// handleFilterKey edita el filtro del sidebar en vivo (captura el teclado).
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.sidebar.clearFilter()
	case tea.KeyEnter:
		m.sidebar.filterActive = false // fija el query
	case tea.KeyBackspace:
		if q := m.sidebar.filterQuery; q != "" {
			m.sidebar.setFilter(q[:len(q)-1])
		}
	case tea.KeyRunes:
		m.sidebar.setFilter(m.sidebar.filterQuery + string(msg.Runes))
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		if m.current.Images != nil {
			m.sidebar.imagesState = imagesLoading
		}
		return m, tea.Batch(m.loadServicesCmd(), m.loadReposCmd())
	case key.Matches(msg, m.keys.Context):
		// abre el overlay de selección de contexto
		m.overlay = newPickerOverlay(m.keys, m.contexts, m.current.Name)
		m.notice = ""
		return m, nil
	case key.Matches(msg, m.keys.Deploy), key.Matches(msg, m.keys.Scale), key.Matches(msg, m.keys.Rollback):
		return m.openAction(msg)
	case key.Matches(msg, m.keys.Filter):
		if m.focus == focusSidebar {
			m.sidebar.filterActive = true
		}
		return m, nil
	}

	if m.focus == focusPanel {
		switch {
		case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.Up):
			// m.events es receptor-valor, pero la copia mutada se preserva porque se retorna via `return m, cmd`.
			cmd := m.events.Update(msg)
			return m, cmd
		}
		// cambiar pestaña: derecha/izquierda (cíclico en ambos sentidos)
		switch {
		case key.Matches(msg, m.keys.Right):
			m.tabs.Next()
		case key.Matches(msg, m.keys.Left):
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
	case key.Matches(msg, m.keys.Enter), key.Matches(msg, m.keys.Space):
		if e, ok := m.sidebar.cursorEntry(); ok && e.Kind == entryHeader {
			m.sidebar.toggle(e.Section)
		}
	}
	return m, m.syncRepoTags()
}

// applyContextSwitch conmuta al contexto recibido en sel; si el provider falla, muestra un notice.
func (m Model) applyContextSwitch(sel config.Context) (tea.Model, tea.Cmd) {
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
	m.sidebar.repoPrefix = sel.RepoPrefix()
	if sel.Images != nil {
		m.sidebar.imagesState = imagesLoading
	}
	m.sidebar.width = m.sidebarW - 1
	m.loading = true
	m.notice = ""
	m.status = ""
	m.focus = focusSidebar
	// reiniciar el estado de watch de deploy para evitar que el loop de poll
	// siga disparándose contra el nuevo deployer con el servicio anterior
	m.deploy.Reset()
	m.events.Reset()
	m.tabs.Active = panel.TabDetails
	m.tagsRepo, m.tags, m.tagsErr, m.tagsLoading = "", nil, "", false
	return m, tea.Batch(m.loadServicesCmd(), m.loadReposCmd())
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
		m.form = newActionForm(actionDeploy, s.Name)
	case key.Matches(msg, m.keys.Scale):
		m.form = newActionForm(actionScale, s.Name)
	case key.Matches(msg, m.keys.Rollback):
		m.form = newActionForm(actionRollback, s.Name)
	}
	m.tabs.Active = panel.TabDetails // el formulario vive en Details
	m.focus = focusPanel
	m.notice = ""
	if key.Matches(msg, m.keys.Deploy) {
		return m, m.loadFormTagsCmd(s.Name)
	}
	return m, nil
}

func (m *Model) runActionCmd(kind actionKind, service, input string) tea.Cmd {
	dep := m.dep
	ctx := m.runCtx
	return func() tea.Msg {
		switch kind {
		case actionRollback:
			return actionDoneMsg{msg: "rolled back " + service, err: dep.Rollback(ctx, service)}
		case actionDeploy:
			// El deploy SIEMPRE va por startDeployCmd (flujo en vivo con eventos).
			// Esta rama solo es alcanzable si un refactor rompe el guard de Enter.
			return actionDoneMsg{err: fmt.Errorf("internal: deploy must go through startDeployCmd")}
		case actionScale:
			n, convErr := strconv.Atoi(input)
			if convErr != nil {
				return actionDoneMsg{err: convErr}
			}
			return actionDoneMsg{msg: "scaled " + service + " to " + input,
				err: dep.Scale(ctx, service, n)}
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

// clickForm resuelve un click con el formulario abierto: activa el botón bajo el
// cursor o no hace nada. La geometría (como la de los botones de Details) solo es
// válida en dos columnas; en una columna el formulario se opera por teclado.
func (m *Model) clickForm(msg tea.MouseMsg) tea.Cmd {
	if m.singleColumn {
		return nil
	}
	// fila superior del formulario: cuerpo del panel (pestañas + línea en blanco)
	// + las filas de DetailsView (0..DetailsButtonLine) → el form empieza después.
	formY0 := topBarHeight + borderTop + 2 + panel.DetailsButtonLine + 1
	row := msg.Y - formY0
	x := msg.X - (m.sidebarW + 2) // contenido del panel: divisor + PaddingLeft
	if idx := m.form.tagAt(row); idx >= 0 {
		m.form.pick = idx
		m.form.input = m.form.visibleTags()[idx].Tag
		return nil
	}
	idx := m.form.buttonAt(row, x)
	if idx < 0 {
		return nil
	}
	done, result := m.form.activateIndex(idx)
	if done {
		m.form = nil
	}
	if r, ok := result.(actionConfirmedMsg); ok {
		return m.applyActionConfirmed(r)
	}
	return nil
}

// openActionKind abre el formulario para una acción (click en botones de Details).
func (m *Model) openActionKind(kind actionKind) tea.Cmd {
	s, ok := m.sidebar.selected()
	if !ok {
		return nil
	}
	m.form = newActionForm(kind, s.Name)
	m.tabs.Active = panel.TabDetails
	m.focus = focusPanel
	m.notice = ""
	if kind == actionDeploy {
		return m.loadFormTagsCmd(s.Name)
	}
	return nil
}

func (m Model) View() string {
	if m.err != nil {
		return render.Danger("error: "+m.err.Error()) + "\n" + render.Dim("press q to quit")
	}
	top := topBar(m.width, m.current.Cloud, m.current.Name, m.current.Cluster, m.current.Writable)
	rule := hrule(m.width)

	if m.overlay != nil {
		return top + "\n" + rule + "\n" + m.overlay.View(m.width, m.bodyH) + "\n" +
			rule + "\n" + bottomBar(m.keys.shortHelp(), m.notice, m.status)
	}

	block := func(w int) lipgloss.Style {
		return lipgloss.NewStyle().Width(w).Height(m.bodyH).PaddingLeft(1)
	}
	header := m.tabs.View()
	if m.sidebar.lastSelected == sectionImages {
		header = render.Brand("TAGS")
	}
	panelBody := header + "\n\n" + m.panelBody()
	var body string
	if m.singleColumn {
		side := block(m.sidebarW).Height(m.bodyH / 2).Render(m.sidebar.view(m.focus == focusSidebar))
		pan := block(m.panelW).Height(m.bodyH - m.bodyH/2 - 1).Render(panelBody)
		body = lipgloss.JoinVertical(lipgloss.Left, side, rule, pan)
	} else {
		side := block(m.sidebarW).Render(m.sidebar.view(m.focus == focusSidebar))
		pan := block(m.panelW).Render(panelBody)
		body = lipgloss.JoinHorizontal(lipgloss.Top, side, vdivider(m.bodyH), pan)
	}
	bottom := bottomBar(m.keys.shortHelp(), m.notice, m.status)
	return top + "\n" + rule + "\n" + body + "\n" + rule + "\n" + bottom
}

func (m Model) panelBody() string {
	if m.sidebar.lastSelected == sectionImages {
		repo, ok := m.sidebar.selectedRepo()
		if !ok {
			return render.Dim("no repository selected")
		}
		switch {
		case m.tagsLoading:
			return render.Dim("loading tags…")
		case m.tagsErr != "":
			return render.Danger("registry error: " + m.tagsErr)
		default:
			short := strings.TrimPrefix(repo, m.current.RepoPrefix())
			return panel.TagsView(short, m.tags, m.deployedTagFor(repo), time.Now())
		}
	}
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
		body := panel.DetailsView(s, m.current.Writable, displayName)
		if m.form != nil {
			body += "\n" + m.form.view()
		}
		return body
	}
}

// deployedTagFor devuelve el tag que corre en el servicio hermano del repo
// (mismo {name} corto en ambos templates); vacío si no hay hermano.
func (m Model) deployedTagFor(repo string) string {
	short := strings.TrimPrefix(repo, m.current.RepoPrefix())
	for _, s := range m.sidebar.services {
		if strings.TrimPrefix(s.Name, m.current.Prefix()) == short {
			return s.Tag
		}
	}
	return ""
}
