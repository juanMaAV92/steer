package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/tui/panel"
	"github.com/stretchr/testify/require"
)

func newTestModel(services []core.ServiceStatus) Model {
	m := New(&coretest.FakeDeployer{Services: services}, "stg-cluster", "stg", true, "")
	m.sidebar.setServices(services)
	m, _ = applySize(m, 120, 40)
	return m
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
	m = mustUpdate(t, m, keyMsg("j"))
	require.Equal(t, 1, m.sidebar.cursor)
	m = mustUpdate(t, m, keyMsg("k"))
	require.Equal(t, 0, m.sidebar.cursor)
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
	ro := New(&coretest.FakeDeployer{Services: sampleServices()}, "prod-cluster", "production", false, "")
	ro.sidebar.setServices(sampleServices())
	ro, _ = applySize(ro, 120, 40)
	for _, key := range []string{"d", "s", "R"} {
		m := mustUpdate(t, ro, keyMsg(key))
		require.NotEqual(t, focusAction, m.focus, "key %q must not open action overlay in read-only", key)
		require.NotEmpty(t, m.notice)
	}
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
	require.Equal(t, 2, m.sidebar.cursor)
	require.Equal(t, focusSidebar, m.focus)
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

func TestDeployFlowFeedsEventsPanel(t *testing.T) {
	fake := &coretest.FakeDeployer{
		Services:        sampleServices(),
		DeploymentValue: core.Deployment{Rollout: "COMPLETED", Running: 2, Desired: 2},
	}
	m := New(fake, "stg-cluster", "stg", true, "")
	m.sidebar.setServices(fake.Services)
	m, _ = applySize(m, 120, 40)

	// abrir deploy del 1er servicio (api) y teclear tag
	m = mustUpdate(t, m, keyMsg("d"))
	require.Equal(t, focusAction, m.focus)
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	// enter ejecuta: devuelve startDeployCmd y salta a Events
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.NotNil(t, cmd)

	started := cmd().(deployStartedMsg)
	require.NoError(t, started.err)
	require.Equal(t, []string{"stg-cluster/api/v2"}, fake.DeployCalls)

	updated, cmd = m.Update(started)
	m = updated.(Model)
	require.NotNil(t, cmd) // primer poll

	poll := cmd().(deployPollMsg)
	updated, _ = m.Update(poll)
	m = updated.(Model)
	require.True(t, m.deployDone)
	require.Contains(t, m.events.View(), "completed")
}
