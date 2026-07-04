package providers

import (
	"context"
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProviderFactoryUnknownCloud(t *testing.T) {
	f := NewProviderFactory()
	_, err := f(context.Background(), config.Context{Name: "x", Cloud: "gcp", Cluster: "c"})
	require.ErrorIs(t, err, ErrProviderNotImplemented)
	require.ErrorContains(t, err, "gcp")
}

func TestProviderFactoryRespectsContextCancel(t *testing.T) {
	f := NewProviderFactory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f(ctx, config.Context{Name: "x", Cloud: "aws", Profile: "p", Cluster: "c"})
	require.Error(t, err) // la carga de sesión debe respetar el ctx cancelado
}
