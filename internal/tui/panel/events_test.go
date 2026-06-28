package panel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventsAppendAndView(t *testing.T) {
	e := NewEvents()
	e.SetSize(40, 10)
	e.AppendLine("[12:00:00] task started")
	e.AppendLine("[12:00:01] task running")
	e.SetStatusLine("Rollout: IN_PROGRESS")
	out := e.View()
	require.Contains(t, out, "task started")
	require.Contains(t, out, "task running")
	require.Contains(t, out, "Rollout: IN_PROGRESS")
}

func TestEventsReset(t *testing.T) {
	e := NewEvents()
	e.SetSize(40, 10)
	e.AppendLine("old line")
	e.Reset()
	require.NotContains(t, e.View(), "old line")
}
