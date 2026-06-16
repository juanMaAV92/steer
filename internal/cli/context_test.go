package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsProduction(t *testing.T) {
	require.True(t, (&AppContext{EnvName: "prod"}).IsProduction())
	require.False(t, (&AppContext{EnvName: "stg"}).IsProduction())
}

func TestRequireWritableBlocksReadOnly(t *testing.T) {
	ro := &AppContext{EnvName: "prod"} // Env.Writable == false por defecto
	require.Error(t, ro.RequireWritable())

	rw := &AppContext{EnvName: "stg"}
	rw.Env.Writable = true
	require.NoError(t, rw.RequireWritable())
}
