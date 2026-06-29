package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCandidatePaths(t *testing.T) {
	got := candidatePaths("/work", "/home/u")
	require.Equal(t, []string{
		"/work/steer.toml",
		"/home/u/.config/steer/steer.toml",
	}, got)
}
