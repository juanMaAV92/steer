// internal/tui/action_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestActionDeployTypingAndReady(t *testing.T) {
	var a action
	a.open(actionDeploy, "api")
	require.True(t, a.active)
	require.False(t, a.ready()) // input vacío
	for _, r := range "v2" {
		a.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	require.Equal(t, "v2", a.input)
	require.True(t, a.ready())
	a.typeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, "v", a.input)
}

func TestActionRollbackAlwaysReadyIgnoresTyping(t *testing.T) {
	var a action
	a.open(actionRollback, "api")
	require.True(t, a.ready())
	a.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Empty(t, a.input) // rollback no acepta input
}

func TestActionCloseDeactivates(t *testing.T) {
	var a action
	a.open(actionScale, "api")
	a.close()
	require.False(t, a.active)
}
