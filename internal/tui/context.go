package tui

import "github.com/juanMaAV92/steer/internal/render"

// topBar renderiza la barra de contexto (cloud · env · cluster · writable).
func topBar(cloud, env, cluster string, writable bool) string {
	state := render.Success("writable ●")
	if !writable {
		state = render.Warn("read-only ○")
	}
	return render.Brand("steer") + render.Dim(" — "+cloud+" · ") + render.Accent(env) +
		render.Dim(" (cluster: "+cluster+") — ") + state
}

// bottomBar muestra ayuda y, si hay, un aviso o estado que tiene prioridad visual.
func bottomBar(help, notice, status string) string {
	switch {
	case notice != "":
		return render.Warn(notice)
	case status != "":
		return render.Success(status)
	default:
		return render.Dim(help)
	}
}
