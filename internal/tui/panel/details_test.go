package panel

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestDetailsViewShowsButtonsWithKeys(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2, Status: "ACTIVE", Tag: "v1.4"}
	out := DetailsView(s, true, "api")
	require.Contains(t, out, "Deploy (d)")
	require.Contains(t, out, "Scale (s)")
	require.Contains(t, out, "Rollback (R)")
	require.Contains(t, out, "[") // estilo de botón
}

func TestDetailsViewReadOnlyHasNoButtons(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 1, Desired: 1}
	out := DetailsView(s, false, "api")
	require.Contains(t, strings.ToLower(out), "read-only")
	require.NotContains(t, out, "Deploy (d)")
}

// TestDetailsViewDisplayName verifica que el displayName se muestra en lugar del Name completo.
func TestDetailsViewDisplayName(t *testing.T) {
	s := core.ServiceStatus{Name: "nao-v2-dev-audit-ms", Running: 2, Desired: 2, Tag: "v1"}
	out := DetailsView(s, true, "audit-ms")
	require.Contains(t, out, "audit-ms")
	require.NotContains(t, out, "nao-v2-dev-audit-ms")
}
