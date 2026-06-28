package panel

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)

// Tab identifica una pestaña del panel derecho.
type Tab int

const (
	TabDetails Tab = iota
	TabEvents
	TabLogs
)

func (t Tab) String() string {
	switch t {
	case TabEvents:
		return "Events"
	case TabLogs:
		return "Logs"
	default:
		return "Details"
	}
}

// Tabs es el estado de la barra de pestañas.
type Tabs struct{ Active Tab }

func (tb *Tabs) Next()     { tb.Active = (tb.Active + 1) % Tab(tb.Count()) }
func (tb *Tabs) Prev()     { tb.Active = (tb.Active - 1 + Tab(tb.Count())) % Tab(tb.Count()) }
func (tb *Tabs) Set(t Tab) { tb.Active = t }
func (tb Tabs) Count() int { return 3 }

// activeTabStyle resalta la pestaña activa con el cian de marca y subrayado.
var activeTabStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color(render.BrandColor)).
	Underline(true)

func (tb Tabs) View() string {
	parts := make([]string, 0, tb.Count())
	for i := 0; i < tb.Count(); i++ {
		label := Tab(i).String()
		if Tab(i) == tb.Active {
			label = activeTabStyle.Render(label)
		} else {
			label = render.Dim(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, "  ")
}
