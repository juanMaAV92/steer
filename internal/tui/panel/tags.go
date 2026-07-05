package panel

import (
	"strings"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// TagsView renderiza la lista de tags de un repo: TAG · AGE · SIZE · DIGEST,
// con "● now" en el tag desplegado por el servicio hermano.
func TagsView(repo string, tags []core.ImageTag, deployed string, now time.Time) string {
	var b strings.Builder
	b.WriteString(render.Bold(repo) + "\n\n")
	if len(tags) == 0 {
		b.WriteString(render.Dim("no images yet"))
		return b.String()
	}
	// ancho de la columna TAG para alinear el resto
	w := 0
	for _, t := range tags {
		if len(t.Tag) > w {
			w = len(t.Tag)
		}
	}
	for _, t := range tags {
		pad := strings.Repeat(" ", w-len(t.Tag)+2)
		line := render.Accent(t.Tag) + pad +
			render.Dim(render.Age(t.PushedAt, now)) + "  " +
			render.Dim(render.Size(t.SizeBytes)) + "  " +
			render.Dim(render.ShortDigest(t.Digest))
		if deployed != "" && t.Tag == deployed {
			line += "  " + render.Success("● now")
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
