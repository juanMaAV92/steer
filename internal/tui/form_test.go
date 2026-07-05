// internal/tui/form_test.go
package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
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
	// buttonRow() declara la fila real de los botones dentro del view
	lines := strings.Split(out, "\n")
	require.Contains(t, stripANSI(lines[f.buttonRow()]), "Confirm")
	// hit-testing: la primera columna del contenido cae en el botón 0
	require.Equal(t, 0, f.buttonAt(f.buttonRow(), formContentX0))
	require.Equal(t, -1, f.buttonAt(f.buttonRow()-1, formContentX0)) // otra fila
	require.Equal(t, -1, f.buttonAt(f.buttonRow(), 0))               // borde de la caja
}

func pickerTags() []core.ImageTag {
	now := time.Now()
	return []core.ImageTag{
		{Tag: "v1.4.2", PushedAt: now.Add(-2 * time.Hour)},
		{Tag: "v1.4.1", PushedAt: now.Add(-72 * time.Hour)},
		{Tag: "v1.3.9", PushedAt: now.Add(-200 * time.Hour)},
	}
}

func TestFormTagsFilterByInput(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	f.setTags(pickerTags())
	require.Len(t, f.visibleTags(), 3)
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v1.4")})
	require.Len(t, f.visibleTags(), 2)
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	require.Empty(t, f.visibleTags())
}

func TestFormMovePickFillsInput(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	f.setTags(pickerTags())
	f.movePick(1)
	require.Equal(t, "v1.4.2", f.input) // primer tag (más reciente)
	f.movePick(1)
	require.Equal(t, "v1.4.1", f.input)
	f.movePick(-1)
	require.Equal(t, "v1.4.2", f.input)
	// teclear resetea el pick y vuelve a filtrar sobre lo tecleado
	f.typeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, -1, f.pick)
}

func TestFormGeometryShiftsWithTags(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.Equal(t, 3, f.buttonRow()) // sin tags: geometría de siempre
	f.setTags(pickerTags())
	require.Equal(t, 3+3, f.buttonRow()) // 3 filas de tags entre prompt y botones
	require.Equal(t, 0, f.tagAt(3))      // primera fila de tag
	require.Equal(t, 2, f.tagAt(5))
	require.Equal(t, -1, f.tagAt(6)) // fila de botones, no tag
	require.Equal(t, 0, f.buttonAt(f.buttonRow(), formContentX0))
	// rollback/scale no muestran picker
	s := newActionForm(actionScale, "api")
	s.setTags(pickerTags())
	require.Equal(t, 3, s.buttonRow())
}

func TestFormStatusRowShiftsGeometry(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.Equal(t, 3, f.buttonRow())
	f.validating = true
	require.Equal(t, 1, f.statusRows())
	require.Equal(t, 4, f.buttonRow()) // la línea de estado empuja los botones
	require.Contains(t, stripANSI(f.view()), "validating tag…")
	f.validating = false
	f.errMsg = "tag not found in nao-v2-shared-api"
	require.Equal(t, 4, f.buttonRow())
	require.Contains(t, stripANSI(f.view()), "tag not found in nao-v2-shared-api")
	// con picker: estado + tags desplazan juntos
	f.setTags(pickerTags())
	require.Equal(t, 4+3, f.buttonRow())
	require.Equal(t, 0, f.tagAt(4)) // los tags empiezan tras la línea de estado
	require.Equal(t, -1, f.tagAt(3))
}
