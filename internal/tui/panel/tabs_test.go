package panel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTabsNextCycles(t *testing.T) {
	tb := Tabs{}
	require.Equal(t, TabDetails, tb.Active)
	tb.Next()
	require.Equal(t, TabEvents, tb.Active)
	tb.Next()
	require.Equal(t, TabLogs, tb.Active)
	tb.Next() // vuelve al inicio
	require.Equal(t, TabDetails, tb.Active)
}

func TestTabsPrevCycles(t *testing.T) {
	tb := Tabs{}
	require.Equal(t, TabDetails, tb.Active)
	tb.Prev() // hacia atrás desde el inicio → última pestaña
	require.Equal(t, TabLogs, tb.Active)
	tb.Prev()
	require.Equal(t, TabEvents, tb.Active)
	tb.Prev()
	require.Equal(t, TabDetails, tb.Active)
}

func TestTabAtColumn(t *testing.T) {
	var tb Tabs
	// layout: "Details"(0-6) "  "(7-8) "Events"(9-14) "  "(15-16) "Logs"(17-20)
	require.Equal(t, int(TabDetails), tb.TabAtColumn(0))
	require.Equal(t, int(TabDetails), tb.TabAtColumn(6))
	require.Equal(t, -1, tb.TabAtColumn(7)) // separador
	require.Equal(t, int(TabEvents), tb.TabAtColumn(9))
	require.Equal(t, int(TabEvents), tb.TabAtColumn(14))
	require.Equal(t, int(TabLogs), tb.TabAtColumn(17))
	require.Equal(t, int(TabLogs), tb.TabAtColumn(20))
	require.Equal(t, -1, tb.TabAtColumn(21)) // fuera de rango
	require.Equal(t, -1, tb.TabAtColumn(-1))
}

func TestTabsViewShowsAllTabs(t *testing.T) {
	tb := Tabs{Active: TabEvents}
	out := tb.View()
	require.Contains(t, out, "Details")
	require.Contains(t, out, "Events")
	require.Contains(t, out, "Logs")
	require.Equal(t, 3, tb.Count())
}
