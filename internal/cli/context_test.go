package cli

import (
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func TestIsProduction(t *testing.T) {
	require.True(t, (&AppContext{Ctx: config.Context{Name: "prod", Writable: false}}).IsProduction())
	require.False(t, (&AppContext{Ctx: config.Context{Name: "stg", Writable: true}}).IsProduction())
}

func TestRequireWritable(t *testing.T) {
	require.NoError(t, (&AppContext{Ctx: config.Context{Writable: true}}).RequireWritable())
	require.Error(t, (&AppContext{Ctx: config.Context{Name: "prod", Writable: false}}).RequireWritable())
}
