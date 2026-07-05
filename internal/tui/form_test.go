// internal/tui/form_test.go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestFormDeployTypingAndReady(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.False(t, f.ready()) // input vacío
	for _, r := range "v2" {
		f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	require.Equal(t, "v2", f.input)
	require.True(t, f.ready())
	f.typeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, "v", f.input)
}

func TestFormRollbackAlwaysReadyIgnoresTyping(t *testing.T) {
	f := newActionForm(actionRollback, "api")
	require.True(t, f.ready())
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Empty(t, f.input) // rollback no acepta input
}

func TestFormMoveFocusWraps(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.Equal(t, 0, f.focus) // inicia en confirmar
	f.moveFocus(1)
	require.Equal(t, 1, f.focus)
	f.moveFocus(1)
	require.Equal(t, 0, f.focus) // wrap adelante
	f.moveFocus(-1)
	require.Equal(t, 1, f.focus) // wrap atrás
}

func TestFormActivateConfirmAndCancel(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	done, res := f.activate() // confirmar sin input → no listo, sigue abierto
	require.False(t, done)
	require.Nil(t, res)
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v2")})
	done, res = f.activate()
	require.True(t, done)
	conf, ok := res.(actionConfirmedMsg)
	require.True(t, ok)
	require.Equal(t, actionDeploy, conf.kind)
	require.Equal(t, "api", conf.service)
	require.Equal(t, "v2", conf.input)

	// Cancel siempre cierra sin resultado, aun sin input
	f2 := newActionForm(actionScale, "api")
	done, res = f2.activateIndex(1)
	require.True(t, done)
	require.Nil(t, res)
}

func TestFormViewAndButtonGeometry(t *testing.T) {
	f := newActionForm(actionRollback, "api")
	out := f.view()
	require.Contains(t, out, "Roll back?")
	require.Contains(t, out, "Confirm (↵)")
	require.Contains(t, out, "Cancel (esc)")
	// formButtonRow declara la fila real de los botones dentro del view
	lines := strings.Split(out, "\n")
	require.Contains(t, stripANSI(lines[formButtonRow]), "Confirm")
	// hit-testing: la primera columna del contenido cae en el botón 0
	require.Equal(t, 0, f.buttonAt(formButtonRow, formContentX0))
	require.Equal(t, -1, f.buttonAt(formButtonRow-1, formContentX0)) // otra fila
	require.Equal(t, -1, f.buttonAt(formButtonRow, 0))               // borde de la caja
}
