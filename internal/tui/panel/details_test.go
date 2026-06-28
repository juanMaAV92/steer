package panel

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestDetailsViewShowsStatsAndActions(t *testing.T) {
	s := core.ServiceStatus{Name: "nao-v2-dev-api", Running: 2, Desired: 2, Pending: 0, Status: "ACTIVE", Tag: "v1.4"}
	out := DetailsView(s, true, "api")
	require.Contains(t, out, "2/2")
	require.Contains(t, out, "ACTIVE")
	require.Contains(t, out, "v1.4")
	require.Contains(t, strings.ToLower(out), "deploy")
	require.Contains(t, strings.ToLower(out), "scale")
	require.Contains(t, strings.ToLower(out), "rollback")
	// el nombre visible es el corto
	require.Contains(t, out, "api")
}

func TestDetailsViewReadOnlyHint(t *testing.T) {
	s := core.ServiceStatus{Name: "nao-v2-dev-api", Running: 1, Desired: 1}
	out := DetailsView(s, false, "api")
	require.Contains(t, strings.ToLower(out), "read-only")
}

// TestDetailsViewDisplayName verifica que el displayName se muestra en lugar del Name completo.
func TestDetailsViewDisplayName(t *testing.T) {
	s := core.ServiceStatus{Name: "nao-v2-dev-audit-ms", Running: 2, Desired: 2, Tag: "v1"}
	out := DetailsView(s, true, "audit-ms")
	require.Contains(t, out, "audit-ms")
	require.NotContains(t, out, "nao-v2-dev-audit-ms")
}
