package cli

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func withFakeDeployer(t *testing.T, fake core.Deployer) {
	t.Helper()
	prev := newDeployerFn
	newDeployerFn = func(_ *AppContext) (core.Deployer, string, error) {
		return fake, "stg-cluster", nil
	}
	t.Cleanup(func() { newDeployerFn = prev })
}

func TestServiceStatusListsServices(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{
		Services: []core.ServiceStatus{
			{Name: "catalog", Running: 2, Desired: 2},
			{Name: "billing", Running: 0, Desired: 1},
		},
	})

	out, err := runRoot(t, "service", "status")
	require.NoError(t, err)
	require.Contains(t, out, "catalog")
	require.Contains(t, out, "billing")
	require.True(t, strings.Contains(out, "2/2"))
}
