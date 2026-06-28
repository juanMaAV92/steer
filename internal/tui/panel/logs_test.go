package panel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogsViewStub(t *testing.T) {
	require.Contains(t, strings.ToLower(LogsView()), "logs")
}
