package panel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogsViewStub(t *testing.T) {
	require.Contains(t, strings.ToLower(LogsView()), "logs")
}

func TestLogsVacioMuestraPlaceholder(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 5)
	require.Contains(t, l.View(), "no logs yet")
}

func TestLogsSetLinesBajaAlFondo(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1", "l2", "l3", "l4", "l5"})
	require.Contains(t, l.View(), "l5") // el fondo (lo más nuevo) queda visible
}

func TestLogsAppendMantieneElFondo(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1", "l2", "l3", "l4", "l5"})
	l.AppendLines([]string{"l6"})
	require.Contains(t, l.View(), "l6") // estaba al fondo → sigue al fondo
}

func TestLogsAppendNoRobaElScroll(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1", "l2", "l3", "l4", "l5"})
	l.vp.GotoTop() // el usuario subió a leer historia
	l.AppendLines([]string{"l6"})
	require.Contains(t, l.View(), "l1")    // la posición no cambia
	require.NotContains(t, l.View(), "l6") // lo nuevo no arrastra la vista
}

func TestLogsReset(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1"})
	l.Reset()
	require.Contains(t, l.View(), "no logs yet")
}
