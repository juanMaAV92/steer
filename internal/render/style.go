package render

import "github.com/charmbracelet/lipgloss"

// Bold resalta texto.
func Bold(s string) string { return lipgloss.NewStyle().Bold(true).Render(s) }

// Dim atenúa texto secundario.
func Dim(s string) string { return lipgloss.NewStyle().Faint(true).Render(s) }

// Success colorea texto en verde y negrita (confirmaciones).
func Success(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render(s)
}

// Danger colorea texto en rojo (errores/avisos fuertes).
func Danger(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(s)
}

// Warn colorea texto en amarillo (atención, en progreso).
func Warn(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render(s)
}
