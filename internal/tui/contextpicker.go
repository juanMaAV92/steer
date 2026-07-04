package tui

import (
	"sort"
	"strings"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/juanMaAV92/steer/internal/render"
)

// contextPicker es el overlay para conmutar de contexto.
type contextPicker struct {
	contexts []config.Context // en orden de presentación (agrupado por cloud)
	cursor   int
}

// newContextPicker ordena los contextos por (cloud, nombre) y posiciona el cursor
// en el contexto actual.
func newContextPicker(contexts []config.Context, current string) contextPicker {
	ordered := make([]config.Context, len(contexts))
	copy(ordered, contexts)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Cloud != ordered[j].Cloud {
			return ordered[i].Cloud < ordered[j].Cloud
		}
		return ordered[i].Name < ordered[j].Name
	})
	cur := 0
	for i, c := range ordered {
		if c.Name == current {
			cur = i
			break
		}
	}
	return contextPicker{contexts: ordered, cursor: cur}
}

func (p *contextPicker) moveDown() {
	if p.cursor < len(p.contexts)-1 {
		p.cursor++
	}
}

func (p *contextPicker) moveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *contextPicker) selectIndex(i int) {
	if i >= 0 && i < len(p.contexts) {
		p.cursor = i
	}
}

func (p contextPicker) selected() (config.Context, bool) {
	if p.cursor < 0 || p.cursor >= len(p.contexts) {
		return config.Context{}, false
	}
	return p.contexts[p.cursor], true
}

// indexAtLine mapea una línea (relativa al inicio del picker, donde la línea 0 es el
// título "Switch context") al índice de contexto que se renderiza en esa línea.
// Replica la estructura de view(): título, y por cada contexto un header de cloud
// cuando cambia de cloud, seguido de la fila del contexto. Devuelve ok=false si la
// línea no corresponde a una fila de contexto (título, header, o fuera de rango).
func (p contextPicker) indexAtLine(line int) (int, bool) {
	cur := 1 // la línea 0 es el título
	lastCloud := ""
	for i, c := range p.contexts {
		if c.Cloud != lastCloud {
			cur++ // línea de header de cloud
			lastCloud = c.Cloud
		}
		if cur == line {
			return i, true
		}
		cur++ // línea de la fila del contexto
	}
	return 0, false
}

func (p contextPicker) view() string {
	var b strings.Builder
	b.WriteString(render.Bold("Switch context") + "\n")
	lastCloud := ""
	for i, c := range p.contexts {
		if c.Cloud != lastCloud {
			b.WriteString(render.Dim(strings.ToUpper(c.Cloud)) + "\n")
			lastCloud = c.Cloud
		}
		cursor := "  "
		if i == p.cursor {
			cursor = render.Accent("> ")
		}
		state := render.Success("writable")
		if !c.Writable {
			state = render.Warn("read-only")
		}
		name := c.Name
		if i == p.cursor {
			name = render.Accent(name)
		}
		extra := ""
		if !providers.IsImplemented(c.Cloud) {
			extra = render.Dim("  (no impl.)")
		}
		b.WriteString(cursor + name + "  " + state + extra + "\n")
	}
	b.WriteString(render.Dim("\n↑↓/click select · enter switch · esc cancel"))
	return b.String()
}
