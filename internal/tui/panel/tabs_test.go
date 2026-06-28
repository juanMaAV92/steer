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

func TestTabsViewShowsAllTabs(t *testing.T) {
	tb := Tabs{Active: TabEvents}
	out := tb.View()
	require.Contains(t, out, "Details")
	require.Contains(t, out, "Events")
	require.Contains(t, out, "Logs")
	require.Equal(t, 3, tb.Count())
}
