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

// tuiLogLines es el tail inicial de la pestaña Logs (mismo default que la CLI).
const tuiLogLines = 100

type actionKind int

const (
	actionRollback actionKind = iota
	actionDeploy
	actionScale
	actionResize
)

// deployState agrupa el estado del watch de deploy en vivo; Reset lo limpia entero.
type deployState struct {
	Active     bool
	Done       bool
	Service    string
	LastID     string
	PullErrors int // eventos de fallo de aprovisionamiento en el rollout actual
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

	logs        panel.Logs
	logsService string // servicio cuyo tail/follow está en pantalla ("" = inactivo)
	logsCursor  string
	logsLoading bool
	logsErr     error
	logsGen     int // generación del follow: invalida páginas/ticks obsoletos

	eventsService string // servicio cuyo histórico está en la pestaña Events
	eventsLastID  string // ID del evento más reciente pintado (dedup del refresh)
	eventsErr     string
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
		keys: defaultKeys(), sidebar: newSidebar(), events: panel.NewEvents(), logs: panel.NewLogs(),
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

// validateTagCmd consulta HasTag para el repo hermano del servicio. Estricta +
// degradable: notFound bloquea; error del registry o sin [images] → skipped.
func (m Model) validateTagCmd(service, tag string) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	short := strings.TrimPrefix(service, m.current.Prefix())
	repo := m.current.RepoName(short)
	return func() tea.Msg {
		reg, err := provider.Registry()
		if err != nil {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagSkipped}
		}
		ok, err := reg.HasTag(ctx, repo, tag)
		if errors.Is(err, core.ErrRepoNotFound) {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagRepoNotFound}
		}
		if err != nil {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagSkipped}
		}
		if !ok {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagNotFound}
		}
		return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagOK}
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

// tailLogsCmd pide el tail inicial de logs del servicio para la sesión gen.
func (m Model) tailLogsCmd(service string, gen int) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		src, err := provider.Logs()
		if err != nil {
			return logsPageMsg{gen: gen, initial: true, err: err}
		}
		page, err := src.TailLogs(ctx, service, tuiLogLines)
		return logsPageMsg{gen: gen, initial: true, page: page, err: err}
	}
}

// followLogsCmd pide las líneas posteriores al cursor para la sesión gen.
func (m Model) followLogsCmd(service, cursor string, gen int) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		src, err := provider.Logs()
		if err != nil {
			return logsPageMsg{gen: gen, err: err}
		}
		page, err := src.FollowLogs(ctx, service, cursor)
		return logsPageMsg{gen: gen, page: page, err: err}
	}
}

// syncLogs arranca/detiene el tail+follow según pestaña y selección. Cambiar
// de servicio, pestaña o contexto resetea el contenido y sube la generación
// (las respuestas y ticks de la sesión anterior se descartan al llegar).
// INVARIANTE: debe llamarse tras CUALQUIER mutación de la pestaña activa o de
// la selección del sidebar (teclado, mouse, o cambios programáticos).
func (m *Model) syncLogs() tea.Cmd {
	sel := ""
	if m.tabs.Active == panel.TabLogs && m.sidebar.lastSelected != sectionImages {
		if s, ok := m.sidebar.selected(); ok {
			sel = s.Name
		}
	}
	if sel == m.logsService {
		return nil
	}
	m.logsGen++
	m.logs.Reset()
	m.logsService, m.logsCursor, m.logsErr = sel, "", nil
	if sel == "" {
		m.logsLoading = false
		return nil
	}
	m.logsLoading = true
	return m.tailLogsCmd(sel, m.logsGen)
}

// logLineView formatea una línea de log para la pestaña Logs.
func logLineView(l core.LogLine) string {
	line := render.Dim(l.At.Format("15:04:05")) + "  "
	if l.Container != "" {
		line += render.Accent("["+l.Container+"]") + "  "
	}
	return line + l.Message
}

// eventLine formatea un evento del servicio para el feed del panel.
func eventLine(e core.ServiceEvent) string {
	line := "[" + e.At.Format("15:04:05") + "] " + e.Message
	if e.IsError {
		return render.Danger(line)
	}
	return render.Dim(line)
}

// loadServiceEventsCmd pide el histórico de eventos del servicio.
func (m Model) loadServiceEventsCmd(service string) tea.Cmd {
	dep := m.dep
	ctx := m.runCtx
	return func() tea.Msg {
		evs, err := dep.ServiceEvents(ctx, service)
		return serviceEventsMsg{service: service, events: evs, err: err}
	}
}

// syncEvents dispara la carga del histórico si la pestaña Events está visible
// para un servicio distinto del cargado. El feed de deploy tiene prioridad:
// activo, o terminado con su servicio aún seleccionado, no se toca.
// INVARIANTE: debe llamarse tras CUALQUIER mutación de la pestaña activa o de
// la selección del sidebar (teclado, mouse, o cambios programáticos).
func (m *Model) syncEvents() tea.Cmd {
	if m.tabs.Active != panel.TabEvents || m.sidebar.lastSelected == sectionImages {
		return nil
	}
	s, ok := m.sidebar.selected()
	if !ok {
		return nil
	}
	if m.deploy.Active {
		return nil
	}
	if m.deploy.Done && m.deploy.Service == s.Name {
		return nil // el resultado del deploy sigue en pantalla
	}
	if m.eventsService == s.Name {
		return nil
	}
	if m.deploy.Done {
		m.deploy.Reset() // feed de otro servicio: se abandona
	}
	m.eventsService = s.Name
	m.eventsLastID = ""
	m.eventsErr = ""
	m.events.Reset()
	return m.loadServiceEventsCmd(s.Name)
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
		m.logs.SetSize(m.panelW-2, m.bodyH/2-2)
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
	m.logs.SetSize(m.panelW-2, m.bodyH-2)
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
		// setServices puede mover la selección (fallback al primer servicio si
		// el elegido desapareció): resincronizar logs/events tras el cambio.
		return m, tea.Batch(m.syncEvents(), m.syncLogs())

	case reposMsg:
		switch {
		case msg.disabled:
			m.sidebar.imagesState = imagesDisabled
		case msg.err != nil:
			if len(m.sidebar.repos) > 0 {
				// conservar la lista cargada: el fallo transitorio va como notice
				m.sidebar.imagesState = imagesReady
				m.notice = "images refresh failed: " + msg.err.Error()
			} else {
				m.sidebar.imagesState = imagesError
				m.sidebar.imagesErr = msg.err.Error()
			}
		default:
			m.sidebar.setRepos(msg.repos)
			// si el repo seleccionado desapareció, limpiar el estado de tags para
			// que una respuesta tardía suya no pase el guard de tagsMsg
			if _, ok := m.sidebar.selectedRepo(); !ok && m.tagsRepo != "" {
				m.tagsRepo, m.tags, m.tagsErr, m.tagsLoading = "", nil, "", false
			}
			// el reload exitoso limpia el aviso del fallo transitorio
			if strings.HasPrefix(m.notice, "images refresh failed") {
				m.notice = ""
			}
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

	case logsPageMsg:
		if msg.gen != m.logsGen {
			return m, nil // respuesta de una sesión de follow abandonada
		}
		m.logsLoading = false
		if msg.err != nil {
			m.logsErr = msg.err
			return m, nil
		}
		m.logsCursor = msg.page.Cursor
		lines := make([]string, 0, len(msg.page.Lines))
		for _, l := range msg.page.Lines {
			lines = append(lines, logLineView(l))
		}
		if msg.initial {
			m.logs.SetLines(lines)
		} else if len(lines) > 0 {
			m.logs.AppendLines(lines)
		}
		return m, logsTickCmd(m.logsGen)

	case logsTickMsg:
		if msg.gen != m.logsGen || m.logsService == "" || m.logsErr != nil {
			return m, nil // la sesión murió: el loop se corta aquí
		}
		return m, m.followLogsCmd(m.logsService, m.logsCursor, m.logsGen)

	case formTagsMsg:
		if m.form != nil && m.form.kind == actionDeploy && m.form.service == msg.service {
			m.form.setTags(msg.tags)
		}
		return m, nil

	case tagValidatedMsg:
		// guard de obsolescencia: el form debe seguir abierto validando ESTE tag
		// (input != tag es defensivo: hoy inalcanzable porque el teclado queda
		// congelado durante la validación; protege a futuros productores externos)
		if m.form == nil || m.form.kind != actionDeploy || !m.form.validating ||
			m.form.service != msg.service || m.form.input != msg.tag {
			return m, nil
		}
		switch msg.verdict {
		case tagNotFound:
			m.form.validating = false
			m.form.errMsg = "tag not found in " + msg.repo
			return m, nil
		case tagRepoNotFound:
			m.form.validating = false
			m.form.errMsg = "repository " + msg.repo + " not found"
			return m, nil
		case tagSkipped:
			m.notice = "registry check skipped — deploying unverified tag"
		}
		m.form = nil
		cmd := m.applyActionConfirmed(actionConfirmedMsg{kind: actionDeploy, service: msg.service, input: msg.tag})
		return m, cmd

	case serviceEventsMsg:
		if msg.service != m.eventsService || m.deploy.Active {
			return m, nil // obsoleto, o el feed de deploy tomó la pestaña
		}
		if msg.err != nil {
			m.eventsErr = msg.err.Error()
			return m, nil
		}
		m.eventsErr = ""
		if len(msg.events) > 0 && msg.events[0].ID == m.eventsLastID {
			return m, nil // sin novedades: no repintar (conserva el scroll)
		}
		m.eventsLastID = ""
		if len(msg.events) > 0 {
			m.eventsLastID = msg.events[0].ID
		}
		m.events.Reset()
		for i := len(msg.events) - 1; i >= 0; i-- { // ascendente: lo nuevo al fondo
			m.events.AppendLine(eventLine(msg.events[i]))
		}
		return m, nil

	case tickMsg:
		cmds := []tea.Cmd{m.loadServicesCmd(), tickCmd()}
		if m.tabs.Active == panel.TabEvents && m.eventsService != "" &&
			!m.deploy.Active && !m.deploy.Done {
			cmds = append(cmds, m.loadServiceEventsCmd(m.eventsService))
		}
		return m, tea.Batch(cmds...)

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
			m.events.AppendLine(eventLine(e))
			if core.IsProvisioningFailure(e.Message) {
				m.deploy.PullErrors++
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
		// 3 fallos de aprovisionamiento = rollout atascado (ECS reintenta para
		// siempre sin circuit breaker y nunca reporta FAILED): cortar el poll.
		// La completación/fallo reportado por ECS gana sobre la heurística.
		if m.deploy.PullErrors >= 3 {
			m.events.AppendLine(render.Danger("✗ deployment stuck: image pull failing — roll back with R"))
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
	if r.kind == actionDeploy || r.kind == actionResize {
		m.focus = focusPanel
		m.tabs.Active = panel.TabEvents
		m.events.Reset()
		m.eventsService, m.eventsLastID, m.eventsErr = "", "", ""
		m.deploy = deployState{Active: true, Service: r.service}
		// la pestaña ya cambió a Events: cortar cualquier follow de logs en curso
		// (syncEvents es inofensivo aquí — no-opea mientras deploy.Active).
		logsCmd, eventsCmd := m.syncLogs(), m.syncEvents()
		if r.kind == actionResize {
			return tea.Batch(logsCmd, eventsCmd, startResizeCmd(m.runCtx, m.dep, r.service, r.resources))
		}
		return tea.Batch(logsCmd, eventsCmd, startDeployCmd(m.runCtx, m.dep, r.service, r.input))
	}
	return m.runActionCmd(r.kind, r.service, r.input)
}

// handleFormKey captura el teclado mientras el formulario de acción está abierto:
// esc cancela, enter activa el botón enfocado, tab/←/→ mueven el foco y el resto
// se teclea en el input. Las teclas globales NO disparan (modo captura).
func (m Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.form.validating {
		if key.Matches(msg, m.keys.Esc) {
			m.closeForm() // esc sigue cancelando; el veredicto llegará obsoleto
		}
		return m, nil
	}
	if m.form.kind == actionResize {
		return m.handleResizeFormKey(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Esc):
		m.closeForm()
	case key.Matches(msg, m.keys.Enter):
		done, result := m.form.activate()
		if r, ok := result.(actionConfirmedMsg); ok && r.kind == actionDeploy {
			// deploy no arranca directo: primero se valida el tag (el form queda abierto)
			m.form.validating = true
			m.form.errMsg = ""
			return m, m.validateTagCmd(r.service, r.input)
		}
		if done {
			if result == nil {
				m.closeForm()
			} else {
				m.form = nil
			}
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

// handleResizeFormKey captura el teclado del formulario de resize: ↑↓ mueven el
// campo activo (cpu → memoria → botones), ←→ cambian el valor del campo activo
// (o el foco confirmar/cancelar en la fila de botones), tab avanza de campo y
// enter confirma solo desde la fila de botones (si no, salta a ella).
func (m Model) handleResizeFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Esc):
		m.closeForm()
	case key.Matches(msg, m.keys.Enter):
		if m.form.resField != 2 {
			m.form.resField = 2
			return m, nil
		}
		done, result := m.form.activate()
		if done {
			if result == nil {
				m.closeForm()
			} else {
				m.form = nil
			}
		}
		if result != nil {
			return m.handleOverlayResult(result)
		}
	case msg.Type == tea.KeyDown:
		m.form.moveResField(1)
	case msg.Type == tea.KeyUp:
		m.form.moveResField(-1)
	case key.Matches(msg, m.keys.Tab):
		m.form.moveResField(1)
	case key.Matches(msg, m.keys.Right):
		m.form.moveResValue(1)
	case key.Matches(msg, m.keys.Left):
		m.form.moveResValue(-1)
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
		if m.tabs.Active == panel.TabLogs {
			return m.logs.Update(msg)
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
			return tea.Batch(m.syncRepoTags(), m.syncEvents(), m.syncLogs())
		}
		return nil
	}
	// click en la fila de botones de acción del panel Details
	// La geometría de botones solo es válida en modo dos columnas; en una columna cae
	// dentro del sidebar y provocaría un misfire que abre el formulario de acción erróneamente.
	// fila de botones de Details en pantalla, derivada del layout real del panel
	detailsButtonRowY := topBarHeight + borderTop + 1 /*pestañas*/ + 1 /*línea en blanco*/ + panel.DetailsButtonLine
	// si hay un repo seleccionado el panel muestra TAGS aunque tabs.Active siga en
	// TabDetails (ver panelBody): esa fila puede caer sobre una fila de tag, no un
	// botón de acción, así que hay que excluir explícitamente ese estado.
	if !m.singleColumn && m.current.Writable && m.tabs.Active == panel.TabDetails &&
		m.sidebar.lastSelected != sectionImages && msg.Y == detailsButtonRowY {
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
	return tea.Batch(m.syncEvents(), m.syncLogs())
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
	// setFilter puede saltar la selección al primer visible: resincronizar.
	return m, tea.Batch(m.syncRepoTags(), m.syncEvents(), m.syncLogs())
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
	case key.Matches(msg, m.keys.Deploy), key.Matches(msg, m.keys.Scale), key.Matches(msg, m.keys.Rollback), key.Matches(msg, m.keys.Resize):
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
			if m.tabs.Active == panel.TabLogs {
				cmd := m.logs.Update(msg)
				return m, cmd
			}
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
		return m, tea.Batch(m.syncEvents(), m.syncLogs())
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
	return m, tea.Batch(m.syncRepoTags(), m.syncEvents(), m.syncLogs())
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
	m.logs.Reset()
	m.logsService, m.logsCursor, m.logsErr, m.logsLoading = "", "", nil, false
	m.logsGen++
	m.eventsService, m.eventsLastID, m.eventsErr = "", "", ""
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
	case key.Matches(msg, m.keys.Resize):
		if s.Resources == (core.Resources{}) {
			m.notice = "task-level resources not set — resize unavailable"
			return m, nil
		}
		m.form = newResizeForm(s.Name, m.dep.ResourceOptions(), s.Resources)
	}
	m.tabs.Active = panel.TabDetails // el formulario vive en Details
	// las acciones son sobre servicios: si había un repo seleccionado (panel en TAGS)
	// hay que volver a Services para que panelBody() renderice Details y el formulario
	// sea visible (si no, queda un formulario invisible que igual captura el teclado).
	m.sidebar.lastSelected = sectionServices
	m.focus = focusPanel
	m.notice = ""
	// el form se abre sobre Details: resincronizar logs/events tras el cambio de pestaña.
	logsCmd, eventsCmd := m.syncLogs(), m.syncEvents()
	if key.Matches(msg, m.keys.Deploy) {
		return m, tea.Batch(logsCmd, eventsCmd, m.loadFormTagsCmd(s.Name))
	}
	return m, tea.Batch(logsCmd, eventsCmd)
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
	case 3:
		return actionResize
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
	if m.form.validating {
		return nil
	}
	// fila superior del formulario: cuerpo del panel (pestañas + línea en blanco)
	// + las filas de DetailsView (0..DetailsButtonLine) → el form empieza después.
	formY0 := topBarHeight + borderTop + 2 + panel.DetailsButtonLine + 1
	row := msg.Y - formY0
	x := msg.X - (m.sidebarW + 2) // contenido del panel: divisor + PaddingLeft
	if fld, idx := m.form.resizeValueAt(row, x); fld >= 0 {
		switch fld {
		case 0:
			prevMem := m.form.resOpts[m.form.cpuIdx].MemoryMiB[m.form.memIdx]
			m.form.cpuIdx = idx
			m.form.memIdx = nearestIdx(m.form.resOpts[m.form.cpuIdx].MemoryMiB, prevMem)
		case 1:
			m.form.memIdx = idx
		}
		m.form.resField = fld
		return nil
	}
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
	if r, ok := result.(actionConfirmedMsg); ok && r.kind == actionDeploy {
		m.form.validating = true
		m.form.errMsg = ""
		return m.validateTagCmd(r.service, r.input)
	}
	if done {
		if result == nil {
			m.closeForm()
		} else {
			m.form = nil
		}
	}
	if r, ok := result.(actionConfirmedMsg); ok {
		return m.applyActionConfirmed(r)
	}
	return nil
}

// closeForm cierra el formulario SIN ejecutar acción y devuelve el panel a TAGS
// si el cursor del sidebar sigue sobre un repo (la acción se abrió desde ahí).
// Los cierres con acción confirmada NO pasan por aquí (van a Events/Details).
func (m *Model) closeForm() {
	m.form = nil
	if e, ok := m.sidebar.cursorEntry(); ok && e.Kind == entryRepo {
		m.sidebar.lastSelected = sectionImages
	}
}

// openActionKind abre el formulario para una acción (click en botones de Details).
func (m *Model) openActionKind(kind actionKind) tea.Cmd {
	s, ok := m.sidebar.selected()
	if !ok {
		return nil
	}
	if kind == actionResize {
		if s.Resources == (core.Resources{}) {
			m.notice = "task-level resources not set — resize unavailable"
			return nil
		}
		m.form = newResizeForm(s.Name, m.dep.ResourceOptions(), s.Resources)
	} else {
		m.form = newActionForm(kind, s.Name)
	}
	m.tabs.Active = panel.TabDetails
	// las acciones son sobre servicios: forzar el panel de vuelta a Services
	// para que el formulario sea visible (ver comentario en openAction).
	m.sidebar.lastSelected = sectionServices
	m.focus = focusPanel
	m.notice = ""
	// el form se abre sobre Details: resincronizar logs/events tras el cambio de pestaña.
	logsCmd, eventsCmd := m.syncLogs(), m.syncEvents()
	if kind == actionDeploy {
		return tea.Batch(logsCmd, eventsCmd, m.loadFormTagsCmd(s.Name))
	}
	return tea.Batch(logsCmd, eventsCmd)
}

func (m Model) View() string {
	if m.err != nil {
		return render.Danger("error: "+providers.Friendly(m.err)) + "\n" + render.Dim("press q to quit")
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
	// defensa en profundidad: lipgloss Height solo RELLENA, nunca trunca. Un
	// contenido más alto que bodyH (p. ej. una tabla de tags larga) haría que el
	// frame exceda la terminal, esta scrollearía y TODAS las coordenadas Y del
	// mouse quedarían corridas. El recorte garantiza frame == alto de terminal.
	body = clampLines(body, m.bodyH)
	bottom := bottomBar(m.keys.shortHelp(), m.notice, m.status)
	return top + "\n" + rule + "\n" + body + "\n" + rule + "\n" + bottom
}

// clampLines recorta s a n líneas como máximo.
func clampLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
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
			return panel.TagsView(m.shortRepo(repo), m.tags, m.deployedTagFor(repo), time.Now())
		}
	}
	s, ok := m.sidebar.selected()
	if !ok {
		return render.Dim("no service selected")
	}
	switch m.tabs.Active {
	case panel.TabEvents:
		if m.eventsErr != "" && !m.deploy.Active && !m.deploy.Done {
			return render.Danger("events error: " + m.eventsErr)
		}
		return m.events.View()
	case panel.TabLogs:
		switch {
		case m.logsLoading:
			return render.Dim("loading logs…")
		case m.logsErr != nil:
			if errors.Is(m.logsErr, core.ErrNoLogSource) {
				return render.Dim(m.logsErr.Error())
			}
			return render.Danger("logs error: " + providers.Friendly(m.logsErr))
		default:
			return m.logs.View()
		}
	default:
		displayName := strings.TrimPrefix(s.Name, m.current.Prefix())
		body := panel.DetailsView(s, m.current.Writable, displayName)
		if m.form != nil {
			body += "\n" + m.form.view()
		}
		return body
	}
}

// shortRepo devuelve el nombre de display del repo (sin el prefijo del contexto).
func (m Model) shortRepo(repo string) string {
	return strings.TrimPrefix(repo, m.current.RepoPrefix())
}

// deployedTagFor devuelve el tag que corre en el servicio hermano del repo
// (mismo {name} corto en ambos templates); vacío si no hay hermano.
func (m Model) deployedTagFor(repo string) string {
	short := m.shortRepo(repo)
	for _, s := range m.sidebar.services {
		if strings.TrimPrefix(s.Name, m.current.Prefix()) == short {
			return s.Tag
		}
	}
	return ""
}
