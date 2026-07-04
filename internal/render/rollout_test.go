package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRolloutContainsState(t *testing.T) {
	for _, s := range []string{"COMPLETED", "FAILED", "IN_PROGRESS"} {
		require.Contains(t, Rollout(s), s)
	}
}
