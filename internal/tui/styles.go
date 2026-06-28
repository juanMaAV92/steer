package tui

import "github.com/charmbracelet/lipgloss"

const (
	sidebarMinWidth       = 24
	singleColumnThreshold = 80
)

// focusedBorder es el borde de un panel con foco (resaltado).
func focusedBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")) // azul
}

// blurredBorder es el borde de un panel sin foco (apagado).
func blurredBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")) // gris
}
