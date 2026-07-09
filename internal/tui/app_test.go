package tui

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/juanMaAV92/steer/internal/tui/panel"
	"github.com/stretchr/testify/require"
)

var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// fakeFactory adapta un core.Deployer fake a una ProviderFactory (para tests).
func fakeFactory(dep core.Deployer) providers.ProviderFactory {
	return func(context.Context, config.Context) (providers.Provider, error) {
		return fakeProvider{dep: dep}, nil
	}
}

// fakeFactoryWithRegistry adapta un core.Deployer y un core.Registry fake a
// una ProviderFactory (para tests de la capacidad de imágenes).
func fakeFactoryWithRegistry(dep core.Deployer, reg core.Registry) providers.ProviderFactory {
	return func(context.Context, config.Context) (providers.Provider, error) {
		return fakeProvider{dep: dep, reg: reg}, nil
	}
}

// fakeFactoryWithLogs adapta un core.Deployer y un core.LogSource fake a una
// ProviderFactory (para tests de la pestaña Logs).
func fakeFactoryWithLogs(dep core.Deployer, src core.LogSource) providers.ProviderFactory {
	return func(context.Context, config.Context) (providers.Provider, error) {
		return fakeProvider{dep: dep, logs: src}, nil
	}
}

// fakeProvider adapta fakes de core al Provider bundle.
type fakeProvider struct {
	dep  core.Deployer
	reg  core.Registry  // nil → capacidad deshabilitada
	logs core.LogSource // nil → sin log source (tests)
}

func (p fakeProvider) Deployer() (core.Deployer, error) { return p.dep, nil }

func (p fakeProvider) Registry() (core.Registry, error) {
	if p.reg == nil {
		return nil, core.ErrNoImagesConfig
	}
	return p.reg, nil
}

func (p fakeProvider) Logs() (core.LogSource, error) {
	if p.logs == nil {
		return nil, core.ErrNoLogSource
	}
	return p.logs, nil
}

// stripANSI elimina secuencias de escape ANSI de una cadena.
func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

func newTestModel(services []core.ServiceStatus) Model {
	fake := &coretest.FakeDeployer{Services: services}
	factory := fakeFactory(fake)
	cur := config.Context{Name: "stg", Cloud: "aws", Cluster: "stg-cluster", Writable: true}
	m := New(context.Background(), factory, []config.Context{cur}, cur)
	m.sidebar.setServices(services)
	m, _ = applySize(m, 120, 40)
	return m
}

// newTestModelWithDeployer es como newTestModel pero con el FakeDeployer inyectado
// (para inspeccionar sus llamadas, p. ej. ResizeCalls).
func newTestModelWithDeployer(t *testing.T, fake *coretest.FakeDeployer) Model {
	t.Helper()
	factory := fakeFactory(fake)
	cur := config.Context{Name: "stg", Cloud: "aws", Cluster: "stg-cluster", Writable: true}
	m := New(context.Background(), factory, []config.Context{cur}, cur)
	m.sidebar.setServices(fake.Services)
	m, _ = applySize(m, 120, 40)
	return m
}

// newTestModelWithRegistry es como newTestModel pero con un registry fake y un
// contexto con bloque [images] (para probar la capacidad IMAGES).
func newTestModelWithRegistry(services []core.ServiceStatus, reg core.Registry) Model {
	fake := &coretest.FakeDeployer{Services: services}
	factory := fakeFactoryWithRegistry(fake, reg)
	cur := config.Context{
		Name: "stg", Cloud: "aws", Cluster: "stg-cluster", Writable: true,
		Images: &config.ImagesConfig{RepoTemplate: "{name}"},
	}
	m := New(context.Background(), factory, []config.Context{cur}, cur)
	m.sidebar.setServices(services)
	m, _ = applySize(m, 120, 40)
	return m
}

// servicesNamed crea servicios mínimos con los nombres dados (para que el repo
// hermano coincida con el template "{name}" de newTestModelWithRegistry).
func servicesNamed(names ...string) []core.ServiceStatus {
	out := make([]core.ServiceStatus, len(names))
	for i, n := range names {
		out[i] = core.ServiceStatus{Name: n, Running: 1, Desired: 1}
	}
	return out
}

func applySize(m Model, w, h int) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(Model), cmd
}

func mustUpdate(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestServicesMsgPopulatesSidebar(t *testing.T) {
	m := newTestModel(nil)
	m = mustUpdate(t, m, servicesMsg{services: sampleServices()})
	require.Len(t, m.sidebar.services, 4)
	require.False(t, m.loading)
}

func TestSidebarKeyboardNavigation(t *testing.T) {
	m := newTestModel(sampleServices())
	// cursor inicial: primer servicio ("api")
	sel, ok := m.sidebar.selected()
	require.True(t, ok)
	require.Equal(t, "api", sel.Name)

	m = mustUpdate(t, m, keyMsg("j"))
	sel, ok = m.sidebar.selected()
	require.True(t, ok)
	require.Equal(t, "cron", sel.Name) // 2º servicio ordenado

	m = mustUpdate(t, m, keyMsg("k"))
	sel, ok = m.sidebar.selected()
	require.True(t, ok)
	require.Equal(t, "api", sel.Name)
}

func TestTabMovesFocusToPanel(t *testing.T) {
	m := newTestModel(sampleServices())
	require.Equal(t, focusSidebar, m.focus)
	m = mustUpdate(t, m, keyMsg("tab"))
	require.Equal(t, focusPanel, m.focus)
}

func TestQuitKeys(t *testing.T) {
	m := newTestModel(nil)
	for _, key := range []string{"q", "ctrl+c"} {
		_, cmd := m.Update(keyMsg(key))
		require.NotNil(t, cmd, "expected quit cmd for %q", key)
	}
}

func TestReadOnlyBlocksActions(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	factory := fakeFactory(fake)
	cur := config.Context{Name: "production", Cloud: "aws", Cluster: "prod-cluster", Writable: false}
	ro := New(context.Background(), factory, []config.Context{cur}, cur)
	ro.sidebar.setServices(sampleServices())
	ro, _ = applySize(ro, 120, 40)
	for _, key := range []string{"d", "s", "R"} {
		m := mustUpdate(t, ro, keyMsg(key))
		require.Nil(t, m.overlay)
		require.NotEmpty(t, m.notice)
	}
}

func TestRunActionCmdRejectsDeploy(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	factory := fakeFactory(fake)
	cur := config.Context{Name: "stg", Cloud: "aws", Cluster: "stg-cluster", Writable: true}
	m := New(context.Background(), factory, []config.Context{cur}, cur)
	m.sidebar.setServices(sampleServices())
	m, _ = applySize(m, 120, 40)
	cmd := m.runActionCmd(actionDeploy, "svc", "v1")
	msg := cmd().(actionDoneMsg)
	require.ErrorContains(t, msg.err, "deploy must go through startDeployCmd")
	require.Empty(t, fake.DeployCalls) // jamás llama a Deploy sin streaming
}

func TestViewRendersWithoutPanic(t *testing.T) {
	m := newTestModel(sampleServices())
	require.NotEmpty(t, m.View())
}

func TestRefreshKeyReloads(t *testing.T) {
	m := newTestModel(sampleServices())
	_, cmd := m.Update(keyMsg("r"))
	require.NotNil(t, cmd) // dispara recarga
}

func TestTickReloadsAndReschedules(t *testing.T) {
	m := newTestModel(nil)
	_, cmd := m.Update(tickMsg{})
	require.NotNil(t, cmd)
}

func TestMouseClickSelectsSidebarService(t *testing.T) {
	m := newTestModel(sampleServices())
	// Derivar la coordenada Y desde el render real, no desde las constantes de producción.
	// Esto hace que el test falle si la geometría de handleMouse no coincide con View().
	out := m.View()
	lines := strings.Split(out, "\n")
	targetName := sampleServices()[1].Name // "web"
	targetY := -1
	for i, line := range lines {
		if strings.Contains(line, targetName) {
			targetY = i
			break
		}
	}
	require.GreaterOrEqual(t, targetY, 0, "service %q not found in rendered output", targetName)

	// click izquierdo en la fila del segundo servicio dentro del sidebar
	click := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      3,
		Y:      targetY,
	}
	m = mustUpdate(t, m, click)
	sel, ok := m.sidebar.selected()
	require.True(t, ok)
	require.Equal(t, "web", sel.Name)
	require.Equal(t, focusSidebar, m.focus)
}

// Click en el header de IMAGES la expande (todo es clickeable).
func TestClickHeaderTogglesSection(t *testing.T) {
	m := newTestModel(sampleServices())
	out := m.View()
	clickY := -1
	for y, line := range strings.Split(out, "\n") {
		if strings.Contains(stripANSI(line), "IMAGES") {
			clickY = y
			break
		}
	}
	require.GreaterOrEqual(t, clickY, 0)
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 3, Y: clickY}
	m = mustUpdate(t, m, click)
	require.Contains(t, stripANSI(m.View()), "configure images in steer.toml") // se expandió
}

// enter/space con el cursor en un header lo togglea.
func TestEnterOnHeaderToggles(t *testing.T) {
	m := newTestModel(sampleServices())
	// navegar hasta el header de IMAGES (semántico, no por conteo de j)
	for i := 0; ; i++ {
		require.Less(t, i, 20, "no se alcanzó el header IMAGES")
		e, ok := m.sidebar.cursorEntry()
		require.True(t, ok)
		if e.Kind == entryHeader && e.Section == sectionImages {
			break
		}
		m = mustUpdate(t, m, keyMsg("j"))
	}
	m = mustUpdate(t, m, keyMsg("enter"))
	e, _ := m.sidebar.cursorEntry()
	require.Equal(t, sectionImages, e.Section) // el toggle recoloca el cursor en SU header
	require.Contains(t, stripANSI(m.View()), "configure images in steer.toml")
}

// TestSelectRepoShowsTagsPanel: seleccionar un repo carga y muestra su tabla de tags
// con el marcador del tag desplegado por el servicio hermano.
func TestSelectRepoShowsTagsPanel(t *testing.T) {
	services := []core.ServiceStatus{
		{Name: "api", Running: 2, Desired: 2, Tag: "v1.0.0"},
	}
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags: map[string][]core.ImageTag{"api": {
			{Tag: "v2", Digest: "sha256:bbbb222222222", SizeBytes: 100 * 1024 * 1024, PushedAt: time.Now().Add(-time.Hour)},
			{Tag: "v1.0.0", Digest: "sha256:aaaa111111111", SizeBytes: 90 * 1024 * 1024, PushedAt: time.Now().Add(-48 * time.Hour)},
		}},
	}
	m := newTestModelWithRegistry(services, reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	// click en el repo (anclado al render)
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	require.NotNil(t, cmd, "seleccionar repo debe disparar la carga de tags")
	m = mustUpdate(t, m, cmd().(tagsMsg))
	out := stripANSI(m.View())
	require.Contains(t, out, "TAGS")
	require.Contains(t, out, "v2")
	require.Contains(t, out, "● now") // v1.0.0 coincide con el tag del servicio api
}

// TestDeployWhileRepoSelectedShowsForm: con un repo seleccionado (panel en TAGS),
// pulsar 'd' debe volver el panel a Details y mostrar el formulario de deploy del
// SERVICIO (las acciones son sobre servicios, no sobre repos) — no debe quedar un
// formulario invisible capturando el teclado mientras el panel sigue en TAGS.
func TestDeployWhileRepoSelectedShowsForm(t *testing.T) {
	services := []core.ServiceStatus{
		{Name: "api", Running: 2, Desired: 2, Tag: "v1.0.0"},
	}
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags:  map[string][]core.ImageTag{"api": {{Tag: "v2", Digest: "sha256:bbbb222222222"}}},
	}
	m := newTestModelWithRegistry(services, reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	require.NotNil(t, cmd)
	m = mustUpdate(t, m, cmd().(tagsMsg))
	require.Contains(t, stripANSI(m.View()), "TAGS") // precondición: panel muestra TAGS

	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form, "d debe abrir el formulario de deploy")

	out := stripANSI(m.View())
	require.Contains(t, out, "image tag:", "el formulario debe ser visible, no invisible")
	require.NotContains(t, out, "TAGS", "el panel debe volver a Details, no seguir en TAGS")
	require.Equal(t, "api", m.form.service)
}

// TestClickTagRowDoesNotOpenAction: con el panel mostrando TAGS, un click en la fila
// que ocupa la geometría del botón de acción de Details (detailsButtonRowY) no debe
// abrir ningún formulario — esa fila pertenece a una fila de tags, no a un botón.
func TestClickTagRowDoesNotOpenAction(t *testing.T) {
	services := []core.ServiceStatus{
		{Name: "api", Running: 2, Desired: 2, Tag: "v1.0.0"},
	}
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags:  map[string][]core.ImageTag{"api": {{Tag: "v2", Digest: "sha256:bbbb222222222"}}},
	}
	m := newTestModelWithRegistry(services, reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	require.NotNil(t, cmd)
	m = mustUpdate(t, m, cmd().(tagsMsg))
	require.Contains(t, stripANSI(m.View()), "TAGS")

	detailsButtonRowY := topBarHeight + borderTop + 1 + 1 + panel.DetailsButtonLine
	var clickPanelX int
	if x, y := findInView(t, m.View(), "v2"); y == detailsButtonRowY {
		clickPanelX = x
	} else {
		clickPanelX = m.sidebarW + 3 // dentro de la zona del panel, en dos columnas
	}
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickPanelX, Y: detailsButtonRowY}
	m = mustUpdate(t, m, click)
	require.Nil(t, m.form, "un click en una fila de tags no debe abrir un formulario de acción")
}

func TestMouseWheelScrollsPanelWhenFocused(t *testing.T) {
	m := newTestModel(sampleServices())
	m.focus = focusPanel
	wheel := tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonWheelDown,
	}
	// no debe panic ni cambiar de servicio
	m = mustUpdate(t, m, wheel)
	require.Equal(t, focusPanel, m.focus)
}

func multiCtxModel(t *testing.T) Model {
	t.Helper()
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	factory := providers.ProviderFactory(func(_ context.Context, c config.Context) (providers.Provider, error) {
		if c.Cloud != "aws" {
			return nil, providers.ErrProviderNotImplemented
		}
		return fakeProvider{dep: fake}, nil
	})
	ctxs := []config.Context{
		{Name: "nao-dev", Cloud: "aws", Cluster: "c1", Writable: true},
		{Name: "nao-prod", Cloud: "aws", Cluster: "c2", Writable: false},
		{Name: "acme-staging", Cloud: "gcp", Cluster: "c3", Writable: true},
	}
	m := New(context.Background(), factory, ctxs, ctxs[0])
	m.sidebar.setServices(sampleServices())
	m, _ = applySize(m, 120, 40)
	return m
}

func TestOpenContextPicker(t *testing.T) {
	m := multiCtxModel(t)
	m = mustUpdate(t, m, keyMsg("c"))
	require.IsType(t, &pickerOverlay{}, m.overlay)
}

// Un click izquierdo en la barra superior (Y=0) abre el selector de contexto.
func TestClickTopBarOpensContextPicker(t *testing.T) {
	m := multiCtxModel(t)
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 5, Y: 0}
	m = mustUpdate(t, m, click)
	require.IsType(t, &pickerOverlay{}, m.overlay)
}

// Click sobre la pestaña "Events" del panel la activa. Anclado al render:
// X e Y se derivan de la posición real del texto "Events" en View().
func TestClickPanelTabSwitches(t *testing.T) {
	m := newTestModel(sampleServices())
	require.Equal(t, panel.TabDetails, m.tabs.Active) // arranca en Details

	out := m.View()
	clickX, clickY := -1, -1
	for y, line := range strings.Split(out, "\n") {
		clean := stripANSI(line)
		if i := strings.Index(clean, "Events"); i >= 0 && strings.Contains(clean, "Details") {
			// X del mouse es columna de celda (runas), no offset de bytes:
			// los bordes │ (U+2502) ocupan 3 bytes pero 1 columna.
			clickX = utf8.RuneCountInString(clean[:i])
			clickY = y
			break
		}
	}
	require.GreaterOrEqual(t, clickX, 0, "no se encontró la pestaña Events en el render")

	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY}
	m = mustUpdate(t, m, click)
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.Equal(t, focusPanel, m.focus)
}

// Click sobre una fila del picker conmuta a ese contexto. Anclado al render:
// la Y del click se deriva de la línea real donde aparece el nombre en View().
func TestClickPickerRowSwitchesContext(t *testing.T) {
	m := multiCtxModel(t)
	m = mustUpdate(t, m, keyMsg("c")) // abrir picker
	require.IsType(t, &pickerOverlay{}, m.overlay)

	out := m.View()
	clickY := -1
	for i, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "nao-prod") {
			clickY = i
			break
		}
	}
	require.GreaterOrEqual(t, clickY, 0, "no se encontró la fila nao-prod en el render")

	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 4, Y: clickY}
	m = mustUpdate(t, m, click)
	require.Equal(t, "nao-prod", m.current.Name)
	require.Equal(t, focusSidebar, m.focus)
}

func TestSwitchToWritableContextReloads(t *testing.T) {
	m := multiCtxModel(t)
	m = mustUpdate(t, m, keyMsg("c"))
	m.overlay.(*pickerOverlay).picker.selectIndex(1) // nao-prod (read-only)
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Equal(t, "nao-prod", m.current.Name)
	require.False(t, m.current.Writable)
	require.Equal(t, focusSidebar, m.focus)
	require.NotNil(t, cmd) // recarga
}

func TestSwitchToNotImplementedShowsNotice(t *testing.T) {
	m := multiCtxModel(t)
	prev := m.current.Name
	m = mustUpdate(t, m, keyMsg("c"))
	// localizar acme-staging (gcp) por nombre
	picker := m.overlay.(*pickerOverlay)
	for i, c := range picker.picker.contexts {
		if c.Name == "acme-staging" {
			picker.picker.selectIndex(i)
		}
	}
	m = mustUpdate(t, m, keyMsg("enter"))
	require.Equal(t, prev, m.current.Name) // no cambió
	require.NotEmpty(t, m.notice)
}

func TestSwitchDuringDeployStopsPollLoop(t *testing.T) {
	m := multiCtxModel(t)
	// simular un deploy activo: estado que deja el handler de enter tras iniciar el watch
	m.deploy = deployState{Active: true, Done: false, Service: "old-svc", LastID: "ev-1"}

	// abrir picker y conmutar a otro contexto AWS distinto del actual (nao-dev)
	m = mustUpdate(t, m, keyMsg("c"))
	require.IsType(t, &pickerOverlay{}, m.overlay)
	picker := m.overlay.(*pickerOverlay)
	for i, c := range picker.picker.contexts {
		if c.Cloud == "aws" && c.Name != m.current.Name {
			picker.picker.selectIndex(i)
			break
		}
	}
	updated, _ := m.Update(keyMsg("enter"))
	m = updated.(Model)

	// el estado de watch debe estar completamente limpio tras el switch
	require.False(t, m.deploy.Active, "deployActive debe ser false tras el switch")
	require.Empty(t, m.deploy.Service, "deployService debe vaciarse tras el switch")
	require.Empty(t, m.deploy.LastID, "deployLastID debe vaciarse tras el switch")

	// un tick de poll no debe reprogramar nada (loop huerfano eliminado)
	_, cmd := m.Update(deployPollTickMsg{})
	require.Nil(t, cmd, "deployPollTickMsg no debe devolver cmd cuando deployActive es false")
}

func TestDeployFlowFeedsEventsPanel(t *testing.T) {
	fake := &coretest.FakeDeployer{
		Services:        sampleServices(),
		DeploymentValue: core.Deployment{Rollout: core.RolloutCompleted, Running: 2, Desired: 2},
	}
	factory := fakeFactory(fake)
	cur := config.Context{Name: "stg", Cloud: "aws", Cluster: "stg-cluster", Writable: true}
	m := New(context.Background(), factory, []config.Context{cur}, cur)
	m.sidebar.setServices(fake.Services)
	m, _ = applySize(m, 120, 40)

	// abrir deploy del 1er servicio (api) y teclear tag
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form)
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	// enter dispara la validación (sin registry → skipped); el veredicto arranca el deploy
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.True(t, m.form.validating)
	require.NotNil(t, cmd)
	updated, cmd = m.Update(cmd().(tagValidatedMsg))
	m = updated.(Model)
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.NotNil(t, cmd)

	started := cmd().(deployStartedMsg)
	require.NoError(t, started.err)
	require.Equal(t, []string{"api/v2"}, fake.DeployCalls)

	updated, cmd = m.Update(started)
	m = updated.(Model)
	require.NotNil(t, cmd) // primer poll

	poll := cmd().(deployPollMsg)
	updated, _ = m.Update(poll)
	m = updated.(Model)
	require.True(t, m.deploy.Done)
	require.Contains(t, m.events.View(), "completed")
}

// TestClickDetailsDeployButton verifica que un click en [ Deploy (d) ] abre el formulario de deploy.
// Anclado al render: la Y y X se derivan del texto real de View().
func TestClickDetailsDeployButton(t *testing.T) {
	m := newTestModel(sampleServices()) // foco sidebar, tab Details, writable
	out := m.View()
	clickX, clickY := -1, -1
	for y, line := range strings.Split(out, "\n") {
		clean := stripANSI(line)
		if i := strings.Index(clean, "Deploy (d)"); i >= 0 {
			clickX = utf8.RuneCountInString(clean[:i]) + 1 // dentro de "[ Deploy..."
			clickY = y
			break
		}
	}
	require.GreaterOrEqual(t, clickX, 0, "no se encontró el botón Deploy en el render")
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY}
	m = mustUpdate(t, m, click)
	require.NotNil(t, m.form)
	require.Equal(t, actionDeploy, m.form.kind)
}

// TestSingleColumnDetailsClickNoMisfire verifica que en modo de una columna, un click en el borde
// derecho del panel a la altura Y=11 NO abre una acción (la geometría de botones de Details solo
// aplica en dos columnas). Con width=79 la pantalla ocupa X 0..78; localX = 78-(75+3)=0 que
// coincidiría con el primer botón si no se añade la guarda !m.singleColumn.
func TestSingleColumnDetailsClickNoMisfire(t *testing.T) {
	m := newTestModel(sampleServices())
	m, _ = applySize(m, 79, 30) // ancho < singleColumnThreshold (80) → una columna
	require.True(t, m.singleColumn)
	// X=78 está en el panel (> sidebarW=75) y localX=0 coincide con el primer botón
	// en modo dos columnas, pero NO debe abrir el formulario en modo una columna.
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 78, Y: 11}
	m = mustUpdate(t, m, click)
	require.Nil(t, m.form)
}

// TestClickDetailsScaleAndRollbackButtons verifica que un click en los botones Scale y Rollback
// del panel Details abre el formulario con el actionKind correcto.
// Anclado al render: la X e Y se derivan del texto real de View().
func TestClickDetailsScaleAndRollbackButtons(t *testing.T) {
	for _, tc := range []struct {
		label string
		kind  actionKind
	}{
		{"Scale (s)", actionScale},
		{"Rollback (R)", actionRollback},
	} {
		tc := tc
		t.Run(tc.label, func(t *testing.T) {
			m := newTestModel(sampleServices())
			out := m.View()
			clickX, clickY := -1, -1
			for y, line := range strings.Split(out, "\n") {
				clean := stripANSI(line)
				if i := strings.Index(clean, tc.label); i >= 0 {
					clickX = utf8.RuneCountInString(clean[:i]) + 1
					clickY = y
					break
				}
			}
			require.GreaterOrEqual(t, clickX, 0, "no se encontró "+tc.label)
			click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY}
			m = mustUpdate(t, m, click)
			require.NotNil(t, m.form)
			require.Equal(t, tc.kind, m.form.kind)
		})
	}
}

// TestReadOnlyDetailsButtonsNoOp verifica que en read-only, el click en la fila de botones no abre acción.
func TestReadOnlyDetailsButtonsNoOp(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	factory := fakeFactory(fake)
	cur := config.Context{Name: "prod", Cloud: "aws", Cluster: "c", Writable: false}
	m := New(context.Background(), factory, []config.Context{cur}, cur)
	m.sidebar.setServices(sampleServices())
	m, _ = applySize(m, 120, 40)
	// click donde estaría la fila de botones; en read-only no hay botones → no-op
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.sidebarW + 5, Y: 11}
	m = mustUpdate(t, m, click)
	require.Nil(t, m.overlay)
	require.Nil(t, m.form)
}

// TestResizeFlowStartsLiveRollout: z abre el form preseleccionado; confirmar entra a Events.
func TestResizeFlowStartsLiveRollout(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{
		{Name: "api", Running: 1, Desired: 1, Resources: core.Resources{CPUMilli: 250, MemoryMiB: 512}},
	}}
	m := newTestModelWithDeployer(t, fake)
	m = mustUpdate(t, m, keyMsg("z"))
	require.NotNil(t, m.form)
	require.Equal(t, actionResize, m.form.kind)
	// ←→ sube el tier de cpu; enter desde botones confirma
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m.form.resField = 2
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd)
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.True(t, m.deploy.Active)
	msg := cmd().(deployStartedMsg)
	require.NoError(t, msg.err)
	require.Len(t, fake.ResizeCalls, 1)
}

// TestResizeUnavailableWithoutResources: sin recursos conocidos, z no abre el form.
func TestResizeUnavailableWithoutResources(t *testing.T) {
	m := newTestModel(sampleServices()) // sampleServices sin Resources
	m = mustUpdate(t, m, keyMsg("z"))
	require.Nil(t, m.form)
	require.Contains(t, m.notice, "resize unavailable")
}

// TestClickResizeValue: click en un valor del picker lo selecciona (anclado al render).
func TestClickResizeValue(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{
		{Name: "api", Running: 1, Desired: 1, Resources: core.Resources{CPUMilli: 250, MemoryMiB: 512}},
	}}
	m := newTestModelWithDeployer(t, fake)
	m = mustUpdate(t, m, keyMsg("z"))
	clickX, clickY := findInView(t, m.View(), "0.5 vCPU")
	m = mustUpdate(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	require.NotNil(t, m.form)
	require.Equal(t, 500, m.form.selectedResources().CPUMilli)
}

func TestClickServiceWithScrolledSidebar(t *testing.T) {
	m := newTestModel(manyServices(40))
	// scrollear el sidebar con la rueda
	for range 4 {
		wheel := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: 3, Y: 5}
		m = mustUpdate(t, m, wheel)
	}
	out := m.View()
	targetY, targetName := -1, ""
	for y, line := range strings.Split(out, "\n") {
		clean := stripANSI(line)
		if strings.Contains(clean, "svc-2") && targetY == -1 { // primer svc-2x visible
			fields := strings.Fields(clean)
			// fila: "●" "svc-2X" "1/1" "—"
			targetY, targetName = y, fields[1]
			break
		}
	}
	require.GreaterOrEqual(t, targetY, 0)
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 3, Y: targetY}
	m = mustUpdate(t, m, click)
	sel, ok := m.sidebar.selected()
	require.True(t, ok)
	require.Equal(t, targetName, sel.Name)
}

// TestFormOpensInDetailsTabAndCapturesKeys: 'd' abre el formulario inline en la
// pestaña Details y el teclado queda capturado (las teclas globales no disparan).
func TestFormOpensInDetailsTabAndCapturesKeys(t *testing.T) {
	m := newTestModel(sampleServices())
	m.tabs.Active = panel.TabEvents // desde otra pestaña
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form)
	require.Equal(t, actionDeploy, m.form.kind)
	require.Equal(t, panel.TabDetails, m.tabs.Active)
	// "q" no cierra la app: se teclea en el input
	updated, cmd := m.Update(keyMsg("q"))
	m = updated.(Model)
	require.Nil(t, cmd)
	require.Equal(t, "q", m.form.input)
}

// TestFormTabMovesFocusEnterActivates: tab enfoca Cancel; enter sobre Cancel cierra sin acción.
func TestFormTabMovesFocusEnterActivates(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	require.NotNil(t, m.form)
	require.Equal(t, 1, m.form.focus)
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.Nil(t, cmd)
}

// TestFormEscCloses: esc cierra el formulario sin emitir acción.
func TestFormEscCloses(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("s"))
	require.NotNil(t, m.form)
	m = mustUpdate(t, m, keyMsg("esc"))
	require.Nil(t, m.form)
}

// TestFormRendersInsideDetailsPanel: el formulario se dibuja bajo los botones de Details.
func TestFormRendersInsideDetailsPanel(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	out := stripANSI(m.View())
	require.Contains(t, out, "image tag:")
	require.Contains(t, out, "Cancel (esc)")
	require.Contains(t, out, "Deploy (d)") // los botones de Details siguen visibles
}

func TestDeployStateReset(t *testing.T) {
	d := deployState{Active: true, Done: true, Service: "svc", LastID: "id"}
	d.Reset()
	require.Equal(t, deployState{}, d)
}

func TestFilterModeCapturesGlobalKeys(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("/")) // activa filtro
	m = mustUpdate(t, m, keyMsg("d")) // debe TECLEAR, no abrir deploy
	require.Nil(t, m.overlay)
	require.Contains(t, stripANSI(m.View()), "/d")
	m = mustUpdate(t, m, keyMsg("esc")) // limpia y sale
	require.NotContains(t, stripANSI(m.View()), "/d")
}

func TestFilterEnterKeepsQuery(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("/"))
	for _, r := range "web" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	m = mustUpdate(t, m, keyMsg("enter"))
	sel, _ := m.sidebar.selected()
	require.Equal(t, "web", sel.Name)
	// tras enter, las teclas globales vuelven a funcionar
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form) // abrió el formulario de deploy
}

// TestRepoVanishOnReloadResetsTags: si el repo seleccionado desaparece en un reload,
// el estado de tags se limpia y un tagsMsg tardío del repo desaparecido se ignora.
func TestRepoVanishOnReloadResetsTags(t *testing.T) {
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags:  map[string][]core.ImageTag{"api": {{Tag: "v1", PushedAt: time.Now()}}},
	}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagsMsg))
	require.Equal(t, "api", m.tagsRepo)
	// reload exitoso SIN el repo → estado de tags limpio
	m = mustUpdate(t, m, reposMsg{repos: nil})
	require.Empty(t, m.tagsRepo)
	require.Nil(t, m.tags)
	// un tagsMsg tardío del repo desaparecido no revive nada
	m = mustUpdate(t, m, tagsMsg{repo: "api", tags: reg.Tags["api"]})
	require.Nil(t, m.tags)
	// cargar repos de nuevo para poder testear fallo con repos cargados
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	// un reload fallido con repos cargados deja el aviso...
	m = mustUpdate(t, m, reposMsg{err: errors.New("throttled")})
	require.Contains(t, m.notice, "images refresh failed")
	// ...y el reload exitoso siguiente lo limpia
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	require.Empty(t, m.notice)
}

// findInView localiza needle en el render y devuelve la coordenada de click
// (columna en runas + 1 para caer dentro de la caja, y la fila). Falla si no aparece.
func findInView(t *testing.T, view, needle string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(view, "\n") {
		clean := stripANSI(line)
		if i := strings.Index(clean, needle); i >= 0 {
			return utf8.RuneCountInString(clean[:i]) + 1, row
		}
	}
	t.Fatalf("no se encontró %q en el render", needle)
	return -1, -1
}

// TestClickFormConfirmButton: click en [ Deploy (↵) ] del formulario confirma y
// arranca el flujo de deploy en vivo. Anclado al render.
func TestClickFormConfirmButton(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	clickX, clickY := findInView(t, m.View(), "Deploy (↵)")
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY}
	updated, cmd := m.Update(click)
	m = updated.(Model)
	require.NotNil(t, m.form, "el form queda abierto validando")
	require.True(t, m.form.validating)
	require.NotNil(t, cmd)
	updated, cmd = m.Update(cmd().(tagValidatedMsg)) // sin registry → skipped
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd, "el veredicto skipped debe devolver startDeployCmd")
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.True(t, m.deploy.Active)
}

// TestClickFormCancelButton: click en [ Cancel (esc) ] cierra sin emitir acción.
func TestClickFormCancelButton(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	clickX, clickY := findInView(t, m.View(), "Cancel (esc)")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	require.Nil(t, m.form)
	require.Nil(t, cmd)
	require.False(t, m.deploy.Active)
}

// TestClickOutsideFormIsNoop: con el formulario abierto, el click fuera no cierra,
// no cambia la selección y no abre el picker (reemplaza a TestClickCancelsActionModal).
func TestClickOutsideFormIsNoop(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form)
	before, _ := m.sidebar.selected()
	for _, click := range []tea.MouseMsg{
		{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 5}, // sidebar
		{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 0}, // top bar
	} {
		m = mustUpdate(t, m, click)
		require.NotNil(t, m.form, "el formulario no debe cerrarse con click fuera")
		require.Nil(t, m.overlay, "el click en la top bar no debe abrir el picker")
	}
	after, _ := m.sidebar.selected()
	require.Equal(t, before.Name, after.Name, "la selección no debe cambiar")
}

// TestWheelStillWorksWithFormOpen: la rueda no cierra ni activa nada con el formulario abierto.
func TestWheelStillWorksWithFormOpen(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	wheel := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: m.sidebarW + 5, Y: 10}
	m = mustUpdate(t, m, wheel)
	require.NotNil(t, m.form)
}

// TestFormSingleColumnClickNoop: en una columna la geometría de botones no aplica;
// ningún click activa ni cierra el formulario (teclado sigue funcionando).
func TestFormSingleColumnClickNoop(t *testing.T) {
	m := newTestModel(sampleServices())
	m, _ = applySize(m, 79, 40)
	require.True(t, m.singleColumn)
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form)
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 40, Y: 25})
	m = updated.(Model)
	require.NotNil(t, m.form)
	require.Nil(t, cmd)
}

// TestSidebarShowsReposFromRegistry: los repos llegan por reposMsg y se ven en IMAGES.
func TestSidebarShowsReposFromRegistry(t *testing.T) {
	reg := &coretest.FakeRegistry{Repos: []core.Repository{{Name: "api"}, {Name: "worker"}}}
	m := newTestModelWithRegistry(sampleServices(), reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	out := stripANSI(m.View())
	require.Contains(t, out, "IMAGES")
	require.Contains(t, out, "api")
	require.Contains(t, out, "worker")
	require.NotContains(t, out, "coming soon")
}

// TestDeployFormShowsAndPicksTags: abrir deploy carga tags; ↓ rellena el input; enter despliega el elegido.
func TestDeployFormShowsAndPicksTags(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	require.NotNil(t, cmd, "abrir deploy debe disparar la carga de tags")
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	require.Contains(t, stripANSI(m.View()), "v9.9")
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, "v9.9", m.form.input)
	// enter valida el tag; el veredicto ok cierra el form y arranca el deploy
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.True(t, m.form.validating)
	updated, cmd = m.Update(cmd().(tagValidatedMsg))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd) // startDeployCmd con el tag elegido
}

// TestDeployFormDegradesWithoutRegistry: sin [images], el formulario es el de siempre.
func TestDeployFormDegradesWithoutRegistry(t *testing.T) {
	m := newTestModel(sampleServices()) // sin registry
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	require.NotNil(t, m.form)
	if cmd != nil {
		m = mustUpdate(t, m, cmd().(formTagsMsg)) // llega vacío
	}
	require.Equal(t, 3, m.form.buttonRow()) // geometría sin picker
	// el flujo teclear+enter sigue intacto (la validación degrada a skipped)
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.True(t, m.form.validating)
	updated, cmd = m.Update(cmd().(tagValidatedMsg))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd)
}

// TestClickFormTagRowFillsInput: click en una fila del picker rellena el input (anclado al render).
func TestClickFormTagRowFillsInput(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	clickX, clickY := findInView(t, m.View(), "v9.9")
	m = mustUpdate(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	require.NotNil(t, m.form, "el click en un tag no cierra el formulario")
	require.Equal(t, "v9.9", m.form.input)
}

// TestDeployValidatesTagNotFound: confirmar con tag inexistente mantiene el form
// abierto con el error; corregir y reconfirmar con tag válido despliega.
func TestDeployValidatesTagNotFound(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	for _, r := range "nope" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	// enter dispara la validación: el form queda abierto validando
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.NotNil(t, m.form, "el form no se cierra hasta el veredicto")
	require.True(t, m.form.validating)
	require.NotNil(t, cmd)
	// enter y clicks quedan inertes mientras valida
	updated, c2 := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, c2)
	require.True(t, m.form.validating)
	// veredicto notFound: form abierto con la línea roja, sin deploy
	m = mustUpdate(t, m, cmd().(tagValidatedMsg))
	require.NotNil(t, m.form)
	require.False(t, m.form.validating)
	require.Contains(t, stripANSI(m.View()), "tag not found in api")
	require.False(t, m.deploy.Active)
	require.Equal(t, []string{"api/nope"}, reg.HasTagCalls)
}

// TestDeployValidatesTagOKStartsDeploy: veredicto ok cierra el form y arranca el flujo.
func TestDeployValidatesTagOKStartsDeploy(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyDown}) // pick v9.9
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.True(t, m.form.validating)
	updated, cmd = m.Update(cmd().(tagValidatedMsg))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd) // startDeployCmd
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.True(t, m.deploy.Active)
}

// TestDeploySkippedWithoutRegistryStillDeploys: sin [images] el check se salta con aviso.
func TestDeploySkippedWithoutRegistryStillDeploys(t *testing.T) {
	m := newTestModel(sampleServices()) // sin registry → Registry() = ErrNoImagesConfig
	m = mustUpdate(t, m, keyMsg("d"))
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.NotNil(t, cmd)
	updated, cmd = m.Update(cmd().(tagValidatedMsg)) // verdict: tagSkipped
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd) // el deploy continúa
	require.True(t, m.deploy.Active)
	require.Contains(t, m.notice, "registry check skipped")
}

// TestStaleTagValidatedIgnored: un veredicto tras esc no revive nada.
func TestStaleTagValidatedIgnored(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	msg := cmd().(tagValidatedMsg)
	m = mustUpdate(t, m, keyMsg("esc")) // cancela durante la validación
	require.Nil(t, m.form)
	updated, c2 := m.Update(msg)
	m = updated.(Model)
	require.Nil(t, m.form)
	require.Nil(t, c2)
	require.False(t, m.deploy.Active)
}

// stuckEvents fabrica n eventos de fallo de aprovisionamiento con IDs únicos.
func stuckEvents(n int) []core.ServiceEvent {
	out := make([]core.ServiceEvent, n)
	for i := range out {
		out[i] = core.ServiceEvent{
			ID:      "ev-" + strconv.Itoa(i),
			At:      time.Now(),
			Message: "(service x) was unable to place a task. Reason: CannotPullContainerError",
			IsError: true,
		}
	}
	return out
}

// TestWatchStuckAfterThreePullErrors: al 3er fallo el poll se detiene y avisa.
func TestWatchStuckAfterThreePullErrors(t *testing.T) {
	m := newTestModel(sampleServices())
	m.deploy = deployState{Active: true, Service: "api"}
	poll := func(evs []core.ServiceEvent) deployPollMsg {
		return deployPollMsg{events: evs, rollout: core.RolloutInProgress, desired: 1}
	}
	// 2 fallos: sigue poll-eando
	updated, cmd := m.Update(poll(stuckEvents(2)))
	m = updated.(Model)
	require.True(t, m.deploy.Active)
	require.Equal(t, 2, m.deploy.PullErrors)
	require.NotNil(t, cmd) // deployTickCmd
	// 3er fallo: STUCK — poll detenido, mensaje visible, R sigue vivo
	updated, cmd = m.Update(poll(stuckEvents(1)))
	m = updated.(Model)
	require.False(t, m.deploy.Active)
	require.True(t, m.deploy.Done)
	require.NotNil(t, cmd) // loadServicesCmd (refresca la lista, como done/failed)
	require.Contains(t, stripANSI(m.events.View()), "deployment stuck")
	// el tick huérfano no reprograma nada
	_, c2 := m.Update(deployPollTickMsg{})
	require.Nil(t, c2)
}

// TestWatchCompletionBeatsStuck: si el rollout completa en el mismo poll que
// acumula el 3er fallo, gana la completación (los fallos eran transitorios).
func TestWatchCompletionBeatsStuck(t *testing.T) {
	m := newTestModel(sampleServices())
	m.deploy = deployState{Active: true, Service: "api", PullErrors: 2}
	msg := deployPollMsg{events: stuckEvents(1), rollout: core.RolloutCompleted,
		running: 1, desired: 1, done: true}
	updated, _ := m.Update(msg)
	m = updated.(Model)
	require.False(t, m.deploy.Active)
	require.True(t, m.deploy.Done)
	out := stripANSI(m.events.View())
	require.Contains(t, out, "completed")
	require.NotContains(t, out, "stuck")
}

// TestWatchEventCounterIgnoresNormalEvents: eventos sanos no suman.
func TestWatchEventCounterIgnoresNormalEvents(t *testing.T) {
	m := newTestModel(sampleServices())
	m.deploy = deployState{Active: true, Service: "api"}
	evs := []core.ServiceEvent{{ID: "a", At: time.Now(), Message: "(service x) has started 1 tasks"}}
	updated, _ := m.Update(deployPollMsg{events: evs, rollout: core.RolloutInProgress})
	m = updated.(Model)
	require.Equal(t, 0, m.deploy.PullErrors)
	require.True(t, m.deploy.Active)
}

// TestDeployBlocksWhenRepoMissing: repo inexistente es respuesta definitiva → bloquea.
func TestDeployBlocksWhenRepoMissing(t *testing.T) {
	reg := &coretest.FakeRegistry{HasTagErr: core.ErrRepoNotFound}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	if cmd != nil {
		m = mustUpdate(t, m, cmd().(formTagsMsg))
	}
	for _, r := range "v1" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagValidatedMsg))
	require.NotNil(t, m.form, "repo inexistente bloquea: el form queda abierto")
	require.Contains(t, stripANSI(m.View()), "repository api not found")
	require.False(t, m.deploy.Active)
}

// TestEscRestoresTagsPanel: cancelar una acción abierta desde un repo vuelve a TAGS.
func TestEscRestoresTagsPanel(t *testing.T) {
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags:  map[string][]core.ImageTag{"api": {{Tag: "v1", PushedAt: time.Now()}}},
	}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagsMsg))
	require.Equal(t, sectionImages, m.sidebar.lastSelected)
	// abrir deploy (panel salta a Details con el form) y cancelar con esc
	updated, _ = m.Update(keyMsg("d"))
	m = updated.(Model)
	require.NotNil(t, m.form)
	require.Equal(t, sectionServices, m.sidebar.lastSelected)
	m = mustUpdate(t, m, keyMsg("esc"))
	require.Nil(t, m.form)
	require.Equal(t, sectionImages, m.sidebar.lastSelected, "esc restaura TAGS: el cursor sigue en el repo")
	require.Contains(t, stripANSI(m.View()), "TAGS")
}

// TestReloadErrorKeepsRepos: un Refresh fallido no borra la lista ya cargada.
func TestReloadErrorKeepsRepos(t *testing.T) {
	reg := &coretest.FakeRegistry{Repos: []core.Repository{{Name: "api"}}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	m = mustUpdate(t, m, reposMsg{err: errors.New("throttled")})
	out := stripANSI(m.View())
	require.Contains(t, out, "▣ api", "los repos cargados siguen visibles")
	require.Contains(t, m.notice, "images refresh failed")
	// sin repos previos, el error sí muestra el estado de sección (comportamiento actual)
	m2 := newTestModelWithRegistry(nil, reg)
	m2 = mustUpdate(t, m2, reposMsg{err: errors.New("boom")})
	m2.sidebar.collapsed[sectionImages] = false
	require.Contains(t, stripANSI(m2.View()), "registry error: boom")
}

// TestClickDetailsButtonsWithFormOpenIsNoop: con el form abierto, el click en la fila
// de botones de Details (visible encima) no reabre otra acción.
func TestClickDetailsButtonsWithFormOpenIsNoop(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	require.Equal(t, actionDeploy, m.form.kind)
	clickX, clickY := findInView(t, m.View(), "Scale (s)")
	m = mustUpdate(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	require.NotNil(t, m.form)
	require.Equal(t, actionDeploy, m.form.kind, "el click no debe cambiar la acción abierta")
}

// TestDeployNotFoundThenRetryOK: e2e del loop corregir-y-reintentar.
func TestDeployNotFoundThenRetryOK(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	for _, r := range "bad" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagValidatedMsg)) // notFound
	require.Contains(t, stripANSI(m.View()), "tag not found")
	// corregir: teclear limpia el error; borrar "bad" y poner el tag bueno
	for range 3 {
		m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	require.Empty(t, m.form.errMsg, "editar limpia la línea de error")
	for _, r := range "v9.9" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	updated, _ = m.Update(cmd().(tagValidatedMsg)) // ok
	m = updated.(Model)
	require.Nil(t, m.form)
	require.True(t, m.deploy.Active)
	require.Equal(t, []string{"api/bad", "api/v9.9"}, reg.HasTagCalls)
}

// TestErrorScreenTeaches: el error de SSO vencido se muestra con su remedio.
func TestErrorScreenTeaches(t *testing.T) {
	m := newTestModel(sampleServices())
	m.err = errors.New("failed to refresh cached credentials, the SSO session has expired")
	out := stripANSI(m.View())
	require.Contains(t, out, "aws sso login")
}
