package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTableContainsHeadersAndRows(t *testing.T) {
	out := Table(
		[]string{"SERVICE", "TAG"},
		[][]string{{"catalog", "v1"}, {"billing", "v2"}},
	)
	require.Contains(t, out, "SERVICE")
	require.Contains(t, out, "TAG")
	require.Contains(t, out, "catalog")
	require.Contains(t, out, "billing")
	require.Contains(t, out, "v1")
	// varias líneas (cabecera + filas + bordes)
	require.Greater(t, strings.Count(out, "\n"), 2)
}

func TestAccentWrapsText(t *testing.T) {
	require.Contains(t, Accent("v1.2.3"), "v1.2.3")
}
