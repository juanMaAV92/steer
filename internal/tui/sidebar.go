package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// sidebar es la columna izquierda con secciones apiladas por dominio.
type sidebar struct {
	services      []core.ServiceStatus
	cursor        int
	focused       bool
	width, height int
	prefix        string // prefijo a ocultar en la visualización (ej. "nao-v2-dev-")
}

func newSidebar() sidebar { return sidebar{} }

// setServices guarda los servicios y los ordena alfabéticamente por nombre de visualización.
func (s *sidebar) setServices(svc []core.ServiceStatus) {
	// copiar para no mutar el slice original
	sorted := make([]core.ServiceStatus, len(svc))
	copy(sorted, svc)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := strings.ToLower(strings.TrimPrefix(sorted[i].Name, s.prefix))
		dj := strings.ToLower(strings.TrimPrefix(sorted[j].Name, s.prefix))
		return di < dj
	})
	s.services = sorted
	if s.cursor >= len(s.services) {
		s.cursor = max(0, len(s.services)-1)
	}
}

func (s *sidebar) moveDown() {
	if s.cursor < len(s.services)-1 {
		s.cursor++
	}
}

func (s *sidebar) moveUp() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *sidebar) selectIndex(i int) {
	if i >= 0 && i < len(s.services) {
		s.cursor = i
	}
}

func (s sidebar) selected() (core.ServiceStatus, bool) {
	if s.cursor < 0 || s.cursor >= len(s.services) {
		return core.ServiceStatus{}, false
	}
	return s.services[s.cursor], true
}

// serviceRowCount es el nº de filas de servicio clicables (mapeo de mouse).
func (s sidebar) serviceRowCount() int { return len(s.services) }

func (s sidebar) view() string {
	var b strings.Builder
	b.WriteString(render.Bold("SERVICES") + render.Dim("  ("+strconv.Itoa(len(s.services))+")") + "\n")
	for i, svc := range s.services {
		cursor := "  "
		if i == s.cursor {
			cursor = render.Accent("> ")
		}
		tag := svc.Tag
		if tag == "" {
			tag = "—"
		}
		displayName := strings.TrimPrefix(svc.Name, s.prefix)
		b.WriteString(cursor + render.Symbol(render.StatusLevel(svc.Running, svc.Desired)) + " " +
			displayName + "  " + strconv.Itoa(svc.Running) + "/" + strconv.Itoa(svc.Desired) +
			"  " + render.Dim(tag) + "\n")
	}
	b.WriteString("\n" + render.Dim("IMAGES (ECR)") + "\n" + render.Dim("  (próximamente)") + "\n")
	b.WriteString("\n" + render.Dim("DATABASES") + "\n" + render.Dim("  (próximamente)") + "\n")
	return b.String()
}
