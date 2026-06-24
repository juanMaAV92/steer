package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTuiCmdExists(t *testing.T) {
	cmd := NewTuiCmd()
	require.Equal(t, "tui", cmd.Use)
	require.NotNil(t, cmd.RunE)
}
