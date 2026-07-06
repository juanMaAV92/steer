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

// DetailsButtonLine debe apuntar a la línea real de los botones en el render.
func TestDetailsButtonLineMatchesRender(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2, Status: "ACTIVE", Tag: "v1"}
	lines := strings.Split(DetailsView(s, true, "api"), "\n")
	require.Greater(t, len(lines), DetailsButtonLine)
	require.Contains(t, lines[DetailsButtonLine], "Deploy (d)")
}

// TestDetailsViewShowsResources verifica que la fila de recursos se renderiza
// correctamente con CPULabel y MemLabel.
func TestDetailsViewShowsResources(t *testing.T) {
	s := core.ServiceStatus{
		Name:      "api",
		Running:   2,
		Desired:   2,
		Resources: core.Resources{CPUMilli: 500, MemoryMiB: 1024},
	}
	out := DetailsView(s, true, "api")
	require.Contains(t, out, "0.5 vCPU")
	require.Contains(t, out, "1 GB")
}

// TestDetailsViewResourcesUnknownShowsDash verifica que cuando Resources es zero,
// la fila de cpu/mem muestra "—".
func TestDetailsViewResourcesUnknownShowsDash(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2}
	out := DetailsView(s, true, "api")
	lines := strings.Split(out, "\n")
	// Buscar la línea que contiene "cpu/mem"
	found := false
	for _, line := range lines {
		if strings.Contains(line, "cpu/mem") {
			require.Contains(t, line, "—")
			found = true
			break
		}
	}
	require.True(t, found, "no se encontró línea cpu/mem en el output")
}

// TestDetailsActionLabelsIncludeResize verifica que DetailsActionLabels incluya
// "Resize (z)" y que se renderice en el output.
func TestDetailsActionLabelsIncludeResize(t *testing.T) {
	require.Len(t, DetailsActionLabels, 4)
	require.Equal(t, "Resize (z)", DetailsActionLabels[3])

	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2}
	out := DetailsView(s, true, "api")
	require.Contains(t, out, "Resize (z)")
}
