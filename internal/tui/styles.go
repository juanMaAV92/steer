package tui

import "github.com/charmbracelet/lipgloss"

const (
	sidebarMinWidth       = 24
	singleColumnThreshold = 80
	colorFocusBorder      = "12"   // azul — panel con foco
	colorBlurredBorder    = "240"  // gris — panel sin foco
)

// focusedBorder es el borde de un panel con foco (resaltado).
func focusedBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorFocusBorder))
}

// blurredBorder es el borde de un panel sin foco (apagado).
func blurredBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBlurredBorder))
}
