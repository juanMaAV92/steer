package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusLevel(t *testing.T) {
	require.Equal(t, LevelOK, StatusLevel(2, 2))
	require.Equal(t, LevelWarn, StatusLevel(1, 2))
	require.Equal(t, LevelError, StatusLevel(0, 2))
	require.Equal(t, LevelOK, StatusLevel(0, 0)) // desired 0 = apagado a propósito
}
