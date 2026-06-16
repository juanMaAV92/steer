package aws

import (
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func TestProfileForEnv(t *testing.T) {
	profile, has := profileFor(config.Environment{Profile: "staging"})
	require.True(t, has)
	require.Equal(t, "staging", profile)
}

func TestProfileForEnvEmpty(t *testing.T) {
	_, has := profileFor(config.Environment{})
	require.False(t, has) // sin profile → cadena de credenciales por defecto
}
