package core_test

import (
	"context"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func TestFakeDeployerImplementsInterface(t *testing.T) {
	var d core.Deployer = &coretest.FakeDeployer{
		Services:        []core.ServiceStatus{{Name: "catalog", Running: 2, Desired: 2}},
		CurrentTagValue: "v1.0.0",
	}
	ctx := context.Background()

	got, err := d.ListServices(ctx, "stg-cluster")
	require.NoError(t, err)
	require.Equal(t, "catalog", got[0].Name)

	tag, err := d.CurrentTag(ctx, "stg-cluster", "catalog")
	require.NoError(t, err)
	require.Equal(t, "v1.0.0", tag)

	require.NoError(t, d.Deploy(ctx, "stg-cluster", "catalog", "v2"))
	require.NoError(t, d.Scale(ctx, "stg-cluster", "catalog", 3))
	require.NoError(t, d.Rollback(ctx, "stg-cluster", "catalog"))

	require.Equal(t, []string{"stg-cluster/catalog/v2"}, d.(*coretest.FakeDeployer).DeployCalls)
	require.Equal(t, []string{"stg-cluster/catalog/3"}, d.(*coretest.FakeDeployer).ScaleCalls)
	require.Equal(t, []string{"stg-cluster/catalog"}, d.(*coretest.FakeDeployer).RollbackCalls)
}
