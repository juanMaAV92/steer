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

func TestDeployNonInteractive(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	withFakeDeployer(t, fake)

	out, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"stg-cluster/catalog/v2"}, fake.DeployCalls)
	require.Contains(t, out, "v1")       // preview muestra tag actual
	require.Contains(t, out, "v2")       // y el objetivo
	require.Contains(t, out, "rollback") // sugiere rollback
}

func TestDeployRequiresServiceAndTag(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{})
	_, err := runRoot(t, "service", "deploy", "-y")
	require.Error(t, err)
}

func TestScaleCommand(t *testing.T) {
	fake := &coretest.FakeDeployer{}
	withFakeDeployer(t, fake)
	_, err := runRoot(t, "service", "scale", "-s", "catalog", "-c", "4", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"stg-cluster/catalog/4"}, fake.ScaleCalls)
}

func TestRollbackCommand(t *testing.T) {
	fake := &coretest.FakeDeployer{}
	withFakeDeployer(t, fake)
	_, err := runRoot(t, "service", "rollback", "-s", "catalog", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"stg-cluster/catalog"}, fake.RollbackCalls)
}
