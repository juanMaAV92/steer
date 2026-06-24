package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func newTestModel(services []core.ServiceStatus) Model {
	return New(&coretest.FakeDeployer{Services: services}, "stg-cluster", "stg")
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
