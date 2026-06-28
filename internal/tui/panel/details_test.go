package panel

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestDetailsViewShowsStatsAndActions(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2, Pending: 0, Status: "ACTIVE", Tag: "v1.4"}
	out := DetailsView(s, true)
	require.Contains(t, out, "2/2")
	require.Contains(t, out, "ACTIVE")
	require.Contains(t, out, "v1.4")
	require.Contains(t, strings.ToLower(out), "deploy")
	require.Contains(t, strings.ToLower(out), "scale")
	require.Contains(t, strings.ToLower(out), "rollback")
}

func TestDetailsViewReadOnlyHint(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 1, Desired: 1}
	out := DetailsView(s, false)
	require.Contains(t, strings.ToLower(out), "read-only")
}
