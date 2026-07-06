package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCPU(t *testing.T) {
	for in, want := range map[string]int{"0.25": 250, "0.5": 500, "1": 1000, "500m": 500, "2": 2000} {
		got, err := parseCPU(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	_, err := parseCPU("abc")
	require.Error(t, err)
}

func TestParseMemory(t *testing.T) {
	for in, want := range map[string]int{"512": 512, "2048": 2048, "2GB": 2048, "0.5GB": 512, "512MB": 512, "2gb": 2048} {
		got, err := parseMemory(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	_, err := parseMemory("mucho")
	require.Error(t, err)
}
