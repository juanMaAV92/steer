package core_test

import (
	"context"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func TestFakeDeployerSatisfiesInterface(t *testing.T) {
	var d core.Deployer = &coretest.FakeDeployer{
		Services: []core.ServiceStatus{{Name: "catalog", Running: 2, Desired: 2}},
	}

	got, err := d.ListServices(context.Background(), "stg-cluster")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "catalog", got[0].Name)

	require.NoError(t, d.Deploy(context.Background(), "stg-cluster", "catalog", "v1"))
}
