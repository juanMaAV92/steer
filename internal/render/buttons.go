package render

import (
	"strings"
	"unicode/utf8"
)

const buttonGap = 2 // columnas entre botones

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
func Buttons(labels []string) string {
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, Accent("[ "+l+" ]"))
	}
	return strings.Join(parts, strings.Repeat(" ", buttonGap))
}

// ButtonAtColumn devuelve el índice del botón cuya caja cubre la columna x (relativa al
// inicio de la fila), o -1 si x cae en un separador o fuera de rango.
func ButtonAtColumn(labels []string, x int) int {
	return LabelAtColumn(labels, 4, buttonGap, x)
}
