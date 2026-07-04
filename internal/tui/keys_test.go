package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestDefaultKeysBound(t *testing.T) {
	k := defaultKeys()
	require.True(t, key.Matches(keyMsg("j"), k.Down))
	require.True(t, key.Matches(keyMsg("k"), k.Up))
	require.True(t, key.Matches(keyMsg("d"), k.Deploy))
	require.True(t, key.Matches(keyMsg("q"), k.Quit))
	require.NotEmpty(t, k.shortHelp())
}

func TestNavAndContextKeysBound(t *testing.T) {
	k := defaultKeys()
	require.True(t, key.Matches(keyMsg("l"), k.Right))
	require.True(t, key.Matches(keyMsg("h"), k.Left))
	require.True(t, key.Matches(keyMsg("c"), k.Context))
}

// keyMsg es el helper compartido de tests del paquete tui.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
