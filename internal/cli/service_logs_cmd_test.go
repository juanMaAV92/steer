package cli

import (
	"context"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/stretchr/testify/require"
)

// withFakeLogSource inyecta una factory cuyo provider expone el LogSource dado
// (nil → core.ErrNoLogSource) sobre un FakeDeployer neutro.
func withFakeLogSource(t *testing.T, src core.LogSource) {
	t.Helper()
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: &coretest.FakeDeployer{}, logs: src}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
}

func TestServiceLogsImprimeTail(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	src := &coretest.FakeLogSource{Pages: []core.LogPage{{
		Lines: []core.LogLine{
			{At: t0, Message: "listening on :8080"},
			{At: t0.Add(time.Second), Message: "GET /health 200"},
		},
		Cursor: "c1",
	}}}
	withFakeLogSource(t, src)

	out, err := runRoot(t, "service", "logs", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "listening on :8080")
	require.Contains(t, out, "GET /health 200")
	require.Equal(t, []string{"api/100"}, src.TailCalls) // default -n 100
}

func TestServiceLogsPrefijoDeContenedor(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	src := &coretest.FakeLogSource{Pages: []core.LogPage{{
		Lines: []core.LogLine{{At: t0, Container: "envoy", Message: "ready"}},
	}}}
	withFakeLogSource(t, src)

	out, err := runRoot(t, "service", "logs", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "[envoy]")
}

func TestServiceLogsRespetaN(t *testing.T) {
	src := &coretest.FakeLogSource{}
	withFakeLogSource(t, src)

	_, err := runRoot(t, "service", "logs", "-s", "api", "-n", "25")
	require.NoError(t, err)
	require.Equal(t, []string{"api/25"}, src.TailCalls)
}

func TestServiceLogsSinLineas(t *testing.T) {
	withFakeLogSource(t, &coretest.FakeLogSource{})
	out, err := runRoot(t, "service", "logs", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "no logs in the last hour")
}

func TestServiceLogsSinLogSource(t *testing.T) {
	withFakeLogSource(t, nil) // fakeProvider.logs nil → core.ErrNoLogSource
	_, err := runRoot(t, "service", "logs", "-s", "api")
	require.ErrorIs(t, err, core.ErrNoLogSource)
}

func TestServiceLogsRequiereService(t *testing.T) {
	withFakeLogSource(t, &coretest.FakeLogSource{})
	_, err := runRoot(t, "service", "logs")
	require.ErrorContains(t, err, "--service")
}
