package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func newTestModel(services []core.ServiceStatus) Model {
	return New(&coretest.FakeDeployer{Services: services}, "stg-cluster", "stg", true)
}

func TestServicesMsgPopulates(t *testing.T) {
	m := newTestModel(nil)
	updated, _ := m.Update(servicesMsg{services: []core.ServiceStatus{
		{Name: "catalog", Running: 2, Desired: 2},
		{Name: "billing", Running: 0, Desired: 1},
	}})
	m = updated.(Model)
	require.Len(t, m.services, 2)
	require.False(t, m.loading)
}

func TestCursorNavigation(t *testing.T) {
	m := newTestModel([]core.ServiceStatus{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	m.services = []core.ServiceStatus{{Name: "a"}, {Name: "b"}, {Name: "c"}}

	down := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")}
	up := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")}

	m = mustUpdate(t, m, down) // 0 -> 1
	require.Equal(t, 1, m.cursor)
	m = mustUpdate(t, m, down) // 1 -> 2
	m = mustUpdate(t, m, down) // 2 -> 2 (clamp)
	require.Equal(t, 2, m.cursor)
	m = mustUpdate(t, m, up) // 2 -> 1
	require.Equal(t, 1, m.cursor)
}

func mustUpdate(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestQuitKeys(t *testing.T) {
	m := newTestModel(nil)
	for _, key := range []string{"q", "ctrl+c"} {
		_, cmd := m.Update(keyMsg(key))
		require.NotNil(t, cmd, "expected quit cmd for %q", key)
	}
}

func TestRefreshKeyReloads(t *testing.T) {
	m := newTestModel([]core.ServiceStatus{{Name: "a"}})
	_, cmd := m.Update(keyMsg("r"))
	require.NotNil(t, cmd) // dispara recarga
}

func TestTickReloadsAndReschedules(t *testing.T) {
	m := newTestModel(nil)
	_, cmd := m.Update(tickMsg{})
	require.NotNil(t, cmd)
}

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func TestEnterOpensDetailEscReturns(t *testing.T) {
	m := newTestModel(nil)
	m.services = []core.ServiceStatus{{Name: "catalog"}}

	m = mustUpdate(t, m, keyMsg("enter"))
	require.Equal(t, viewDetail, m.view)

	m = mustUpdate(t, m, keyMsg("esc"))
	require.Equal(t, viewList, m.view)
}

func TestEnterOnEmptyListDoesNothing(t *testing.T) {
	m := newTestModel(nil)
	m = mustUpdate(t, m, keyMsg("enter"))
	require.Equal(t, viewList, m.view)
}

func TestRollbackConfirmAndExecute(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{{Name: "catalog"}}}
	m := New(fake, "stg-cluster", "stg", true)
	m.services = fake.Services

	// 'R' abre confirmación de rollback
	m = mustUpdate(t, m, keyMsg("R"))
	require.Equal(t, viewConfirm, m.view)
	require.Equal(t, actionRollback, m.action.kind)

	// enter ejecuta → devuelve un cmd que llama al deployer
	_, cmd := m.Update(keyMsg("enter"))
	require.NotNil(t, cmd)
	msg := cmd()
	done, ok := msg.(actionDoneMsg)
	require.True(t, ok)
	require.NoError(t, done.err)
	require.Equal(t, []string{"stg-cluster/catalog"}, fake.RollbackCalls)
}

func TestConfirmEscCancels(t *testing.T) {
	m := newTestModel([]core.ServiceStatus{{Name: "catalog"}})
	m.services = []core.ServiceStatus{{Name: "catalog"}}
	m = mustUpdate(t, m, keyMsg("R"))
	m = mustUpdate(t, m, keyMsg("esc"))
	require.Equal(t, viewList, m.view)
}

func TestDeployInputAndExecute(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{{Name: "catalog"}}}
	m := New(fake, "stg-cluster", "stg", true)
	m.services = fake.Services

	m = mustUpdate(t, m, keyMsg("d")) // abre input de deploy
	require.Equal(t, viewConfirm, m.view)
	require.Equal(t, actionDeploy, m.action.kind)

	for _, r := range "v2" { // teclea el tag
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	require.Equal(t, "v2", m.action.input)

	_, cmd := m.Update(keyMsg("enter"))
	require.NotNil(t, cmd)
	done := cmd().(actionDoneMsg)
	require.NoError(t, done.err)
	require.Equal(t, []string{"stg-cluster/catalog/v2"}, fake.DeployCalls)
}

func TestScaleInputAndExecute(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{{Name: "catalog"}}}
	m := New(fake, "stg-cluster", "stg", true)
	m.services = fake.Services

	m = mustUpdate(t, m, keyMsg("s"))
	for _, r := range "3" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	_, cmd := m.Update(keyMsg("enter"))
	done := cmd().(actionDoneMsg)
	require.NoError(t, done.err)
	require.Equal(t, []string{"stg-cluster/catalog/3"}, fake.ScaleCalls)
}

func TestInputBackspace(t *testing.T) {
	m := newTestModel([]core.ServiceStatus{{Name: "catalog"}})
	m.services = []core.ServiceStatus{{Name: "catalog"}}
	m = mustUpdate(t, m, keyMsg("d"))
	for _, r := range "v22" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, "v2", m.action.input)
}

func TestReadOnlyBlocksActions(t *testing.T) {
	ro := New(&coretest.FakeDeployer{Services: []core.ServiceStatus{{Name: "catalog"}}}, "prod-cluster", "production", false)
	ro.services = []core.ServiceStatus{{Name: "catalog"}}

	for _, key := range []string{"d", "s", "R"} {
		m := mustUpdate(t, ro, keyMsg(key))
		require.Equal(t, viewList, m.view, "key %q must not open confirm in read-only env", key)
		require.NotEmpty(t, m.notice)
	}
}
