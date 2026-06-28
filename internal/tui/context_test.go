package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopBarShowsContext(t *testing.T) {
	out := topBar("aws", "staging", "staging-cluster", true)
	require.Contains(t, out, "aws")
	require.Contains(t, out, "staging")
	require.Contains(t, out, "staging-cluster")
	require.Contains(t, strings.ToLower(out), "writable")
}

func TestTopBarReadOnly(t *testing.T) {
	out := topBar("aws", "prod", "prod-cluster", false)
	require.Contains(t, strings.ToLower(out), "read-only")
}

func TestBottomBarShowsNoticeOverHelp(t *testing.T) {
	out := bottomBar("help text", "blocked!", "")
	require.Contains(t, out, "blocked!")
}
