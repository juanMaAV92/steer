package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func TestServiceEventsMuestraAscendente(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	// el deployer entrega más recientes primero (contrato de ServiceEvents)
	withFakeDeployer(t, &coretest.FakeDeployer{Events: []core.ServiceEvent{
		{ID: "2", At: t0.Add(time.Minute), Message: "reached steady state"},
		{ID: "1", At: t0, Message: "started 2 tasks", IsError: false},
	}})

	out, err := runRoot(t, "service", "events", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "TIME")
	require.Contains(t, out, "MESSAGE")
	// ascendente: lo más reciente al final, junto al prompt
	require.Less(t, strings.Index(out, "started 2 tasks"), strings.Index(out, "reached steady state"))
}

func TestServiceEventsRecortaAVeinte(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	evs := make([]core.ServiceEvent, 25)
	for i := range evs {
		// más recientes primero: el índice 0 es el más nuevo
		evs[i] = core.ServiceEvent{ID: string(rune('a' + i)), At: t0.Add(-time.Duration(i) * time.Minute), Message: "event-" + string(rune('a'+i))}
	}
	withFakeDeployer(t, &coretest.FakeDeployer{Events: evs})

	out, err := runRoot(t, "service", "events", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "event-a")    // el más reciente entra
	require.NotContains(t, out, "event-y") // el 25º (más viejo) queda fuera
}

func TestServiceEventsVacio(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{})
	out, err := runRoot(t, "service", "events", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "no events")
}

func TestServiceEventsRequiereService(t *testing.T) {
	_, err := runRootWithFake(t, "service", "events")
	require.ErrorContains(t, err, "--service")
}
