package render

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
)

const buttonGap = 2 // columnas entre botones

// SelectionBarColor es el fondo de la barra de selección (cursor del sidebar,
// botón enfocado del formulario de acción).
const SelectionBarColor = "#0c3a44"

// focusedButton pinta la caja del botón enfocado con la barra de selección.
var focusedButton = lipgloss.NewStyle().
	Foreground(lipgloss.Color(BrandColor)).
	Background(lipgloss.Color(SelectionBarColor)).
	Bold(true)

// LabelAtColumn es la primitiva de hit-testing por columna: devuelve el índice de la
// etiqueta cuya franja (ancho en runas + pad) cubre la columna x, con gap columnas
// entre etiquetas; -1 si x cae en un separador o fuera de rango.
func LabelAtColumn(labels []string, pad, gap, x int) int {
	col := 0
	for i, l := range labels {
		w := utf8.RuneCountInString(l) + pad
		if x >= col && x < col+w {
			return i
		}
		col += w + gap
	}
	return -1
}

// Buttons renderiza una fila de botones "[ label ]" en cian de marca, separados por
// buttonGap espacios. Las etiquetas se muestran tal cual (ASCII o con glifos).
func Buttons(labels []string) string { return ButtonsWithFocus(labels, -1) }

// ButtonsWithFocus renderiza la fila como Buttons resaltando labels[focus] con la
// barra de selección; el ancho de cada caja no cambia con el foco, así el
// hit-testing de ButtonAtColumn vale para ambas variantes. focus fuera de rango
// no resalta ninguno.
func ButtonsWithFocus(labels []string, focus int) string {
	parts := make([]string, 0, len(labels))
	for i, l := range labels {
		box := "[ " + l + " ]"
		if i == focus {
			parts = append(parts, focusedButton.Render(box))
		} else {
			parts = append(parts, Accent(box))
		}
	}
	return strings.Join(parts, strings.Repeat(" ", buttonGap))
}

// ButtonAtColumn devuelve el índice del botón cuya caja cubre la columna x (relativa al
// inicio de la fila), o -1 si x cae en un separador o fuera de rango.
func ButtonAtColumn(labels []string, x int) int {
	return LabelAtColumn(labels, 4, buttonGap, x)
}
