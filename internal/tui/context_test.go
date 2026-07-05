package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func TestTopBarShowsContext(t *testing.T) {
	out := topBar(100, "aws", "staging", "staging-cluster", true)
	require.Contains(t, out, "aws")
	require.Contains(t, out, "staging")
	require.Contains(t, out, "staging-cluster")
	require.Contains(t, strings.ToLower(out), "writable")
	require.Contains(t, out, brandIcon)
}

func TestTopBarReadOnly(t *testing.T) {
	out := topBar(100, "aws", "prod", "prod-cluster", false)
	require.Contains(t, strings.ToLower(out), "read-only")
}

// El estado queda alineado a la derecha: el ancho de display es exactamente width.
func TestTopBarRightAlignsState(t *testing.T) {
	out := topBar(100, "aws", "dev", "c", true)
	require.Equal(t, 100, lipgloss.Width(out))
	require.True(t, strings.HasSuffix(stripANSI(out), "writable ●"))
}

func TestHruleAndVdivider(t *testing.T) {
	require.Equal(t, 10, lipgloss.Width(hrule(10)))
	require.Equal(t, "", hrule(0))
	d := vdivider(3)
	require.Equal(t, 3, strings.Count(d, "│"))
	require.Equal(t, 2, strings.Count(d, "\n"))
}

func TestBottomBarShowsNoticeOverHelp(t *testing.T) {
	out := bottomBar("help text", "blocked!", "")
	require.Contains(t, out, "blocked!")
}
