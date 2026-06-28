package panel

import (
	"strconv"
	"strings"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// DetailsView renderiza la pestaña Details con stats y la fila de acciones.
// displayName es el nombre de visualización (sin prefijo de entorno).
func DetailsView(s core.ServiceStatus, writable bool, displayName string) string {
	var b strings.Builder
	b.WriteString(render.Bold(displayName) + "\n\n")
	b.WriteString("running   " + strconv.Itoa(s.Running) + "/" + strconv.Itoa(s.Desired) + "\n")
	b.WriteString("pending   " + strconv.Itoa(s.Pending) + "\n")
	b.WriteString("status    " + s.Status + "\n")
	tag := s.Tag
	if tag == "" {
		tag = "—"
	}
	b.WriteString("tag       " + render.Accent(tag) + "\n\n")
	if writable {
		b.WriteString(render.Dim("[d] deploy  [s] scale  [R] rollback"))
	} else {
		b.WriteString(render.Warn("read-only environment — actions disabled"))
	}
	return b.String()
}
