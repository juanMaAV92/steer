package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsProvisioningFailure(t *testing.T) {
	require.True(t, IsProvisioningFailure("(service x) was unable to place a task. Reason: CannotPullContainerError: pull image manifest has been retried"))
	require.True(t, IsProvisioningFailure("CANNOTPULLCONTAINERERROR: image not found")) // case-insensitive
	require.True(t, IsProvisioningFailure("(service x) WAS UNABLE TO PLACE A TASK"))
	require.False(t, IsProvisioningFailure("(service x) has started 1 tasks"))
	require.False(t, IsProvisioningFailure("(service x) has reached a steady state."))
	require.False(t, IsProvisioningFailure(""))
}
