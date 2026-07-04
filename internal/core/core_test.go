package core_test

import (
	"context"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func TestRolloutStateConstants(t *testing.T) {
	require.Equal(t, core.RolloutState("COMPLETED"), core.RolloutCompleted)
	require.Equal(t, core.RolloutState("FAILED"), core.RolloutFailed)
	require.Equal(t, core.RolloutState("IN_PROGRESS"), core.RolloutInProgress)
}

func TestFakeDeployerImplementsInterface(t *testing.T) {
	var d core.Deployer = &coretest.FakeDeployer{
		Services:        []core.ServiceStatus{{Name: "catalog", Running: 2, Desired: 2}},
		CurrentTagValue: "v1.0.0",
	}
	ctx := context.Background()

	got, err := d.ListServices(ctx)
	require.NoError(t, err)
	require.Equal(t, "catalog", got[0].Name)

	tag, err := d.CurrentTag(ctx, "catalog")
	require.NoError(t, err)
	require.Equal(t, "v1.0.0", tag)

	require.NoError(t, d.Deploy(ctx, "catalog", "v2", nil))
	require.NoError(t, d.Scale(ctx, "catalog", 3))
	require.NoError(t, d.Rollback(ctx, "catalog"))

	require.Equal(t, []string{"catalog/v2"}, d.(*coretest.FakeDeployer).DeployCalls)
	require.Equal(t, []string{"catalog/3"}, d.(*coretest.FakeDeployer).ScaleCalls)
	require.Equal(t, []string{"catalog"}, d.(*coretest.FakeDeployer).RollbackCalls)
}
