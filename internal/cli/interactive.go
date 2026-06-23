package cli

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/juanMaAV92/steer/internal/core"
)

// option es un par etiqueta/valor para los pickers.
type option struct {
	Label string
	Value string
}

// serviceOptions construye las opciones del picker a partir del estado de servicios.
func serviceOptions(services []core.ServiceStatus) []option {
	opts := make([]option, 0, len(services))
	for _, s := range services {
		opts = append(opts, option{
			Label: fmt.Sprintf("%s  %d/%d", s.Name, s.Running, s.Desired),
			Value: s.Name,
		})
	}
	return opts
}

// pickServiceAndTag muestra los formularios y devuelve servicio elegido y tag.
// Devuelve ok=false si el usuario cancela.
func pickServiceAndTag(opts []option) (service, tag string, ok bool, err error) {
	huhOpts := make([]huh.Option[string], 0, len(opts))
	for _, o := range opts {
		huhOpts = append(huhOpts, huh.NewOption(o.Label, o.Value))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Service").Options(huhOpts...).Value(&service),
		huh.NewInput().Title("Image tag").Value(&tag),
	))
	if err := form.Run(); err != nil {
		return "", "", false, err
	}
	if service == "" || tag == "" {
		return "", "", false, nil
	}
	return service, tag, true, nil
}
