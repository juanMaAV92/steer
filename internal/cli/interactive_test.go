package cli

import (
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestServiceOptionsLabels(t *testing.T) {
	opts := serviceOptions([]core.ServiceStatus{
		{Name: "catalog", Running: 2, Desired: 2},
		{Name: "billing", Running: 0, Desired: 1},
	})
	require.Len(t, opts, 2)
	require.Equal(t, "catalog", opts[0].Value)
	require.Contains(t, opts[0].Label, "catalog")
	require.Contains(t, opts[0].Label, "2/2")
}
