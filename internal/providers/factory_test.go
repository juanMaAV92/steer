package providers

import (
	"errors"
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFactoryUnknownCloud(t *testing.T) {
	f := NewDeployerFactory()
	_, err := f(config.Context{Name: "x", Cloud: "gcp", Cluster: "c"})
	require.ErrorIs(t, err, ErrProviderNotImplemented)
	require.ErrorContains(t, err, "gcp")
}

func TestFactoryAzureUnknownCloud(t *testing.T) {
	f := NewDeployerFactory()
	_, err := f(config.Context{Name: "x", Cloud: "azure", Cluster: "c"})
	require.True(t, errors.Is(err, ErrProviderNotImplemented))
}
