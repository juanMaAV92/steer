package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestButtonsContainsLabels(t *testing.T) {
	out := Buttons([]string{"Deploy (d)", "Scale (s)"})
	require.Contains(t, out, "Deploy (d)")
	require.Contains(t, out, "Scale (s)")
	require.Contains(t, out, "[")
	require.Contains(t, out, "]")
}

func TestButtonAtColumn(t *testing.T) {
	labels := []string{"Deploy (d)", "Scale (s)", "Rollback (R)"}
	// anchos: "Deploy (d)"=10 -> caja 14 (cols 0..13); sep 2 (14,15);
	// "Scale (s)"=9 -> caja 13 (cols 16..28); sep 2 (29,30);
	// "Rollback (R)"=12 -> caja 16 (cols 31..46)
	require.Equal(t, 0, ButtonAtColumn(labels, 0))
	require.Equal(t, 0, ButtonAtColumn(labels, 13))
	require.Equal(t, -1, ButtonAtColumn(labels, 14)) // separador
	require.Equal(t, 1, ButtonAtColumn(labels, 16))
	require.Equal(t, 1, ButtonAtColumn(labels, 28))
	require.Equal(t, 2, ButtonAtColumn(labels, 31))
	require.Equal(t, 2, ButtonAtColumn(labels, 46))
	require.Equal(t, -1, ButtonAtColumn(labels, 47)) // fuera
	require.Equal(t, -1, ButtonAtColumn(labels, -1))
}

func TestButtonAtColumnMultibyte(t *testing.T) {
	// "Deploy (↵)" tiene una runa multibyte; el ancho debe contarse por runas (10), caja 14.
	labels := []string{"Deploy (↵)", "Cancel (esc)"}
	require.Equal(t, 0, ButtonAtColumn(labels, 0))
	require.Equal(t, 0, ButtonAtColumn(labels, 13))
	require.Equal(t, 1, ButtonAtColumn(labels, 16))
	_ = strings.TrimSpace
}
