package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)

const (
	sidebarMinWidth       = 24
	singleColumnThreshold = 80
	colorFocusBorder      = render.BrandColor // cian de marca — panel con foco
	colorBlurredBorder    = "240"             // gris — panel sin foco
	colorSelectionBar     = "#0c3a44"         // cian oscuro — barra de fila seleccionada
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
