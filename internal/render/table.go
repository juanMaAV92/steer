package render

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Table renderiza una tabla alineada con cabeceras en negrita y bordes.
func Table(headers []string, rows [][]string) string {
	t := table.New().
		Border(lipgloss.RoundedBorder()).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, _ int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	return t.String()
}

// Accent colorea texto en verde (tags y valores destacados).
func Accent(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(s)
}
