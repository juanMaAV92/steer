// Package render contiene helpers de presentación compartidos por CLI y TUI.
package render

import "github.com/charmbracelet/lipgloss"

// Level clasifica la salud de un recurso.
type Level int

const (
	LevelOK Level = iota
	LevelWarn
	LevelError
)

// StatusLevel deriva el nivel a partir de réplicas corriendo vs. deseadas.
func StatusLevel(running, desired int) Level {
	switch {
	case desired == 0 || running >= desired:
		return LevelOK
	case running == 0:
		return LevelError
	default:
		return LevelWarn
	}
}

// Symbol devuelve un punto coloreado según el nivel.
func Symbol(l Level) string {
	colors := map[Level]string{
		LevelOK:    "10", // verde
		LevelWarn:  "11", // amarillo
		LevelError: "9",  // rojo
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colors[l])).Render("●")
}
