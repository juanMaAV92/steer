package wizard

import (
	"context"
	"fmt"
	"sort"

	"github.com/charmbracelet/huh"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/juanMaAV92/steer/internal/providers/aws"
)

// aws.Detector debe satisfacer wizard.Detector: única fuente de verdad de la
// interfaz (antes vivía al revés, en aws — eso generaba un ciclo de imports
// con este paquete, que necesita providers.IsImplemented/Friendly).
var _ Detector = (*aws.Detector)(nil)

// cloudOption es un cloud ofrecido en el picker inicial ("aws" hoy es el único
// implementado; gcp/azure aparecen deshabilitados hasta que tengan provider).
type cloudOption struct {
	id, label string
}

var cloudOptions = []cloudOption{
	{id: "aws", label: "AWS"},
	{id: "gcp", label: "GCP"},
	{id: "azure", label: "Azure"},
}

// Run ejecuta el wizard interactivo (huh) de onboarding. existing=nil arranca
// una config desde cero; con existing conserva los contextos previos (modo
// "add"). Run NO toca disco: devuelve la config final y la ruta destino
// elegida — el caller decide cuándo escribir (Write) y corre el smoke test
// posterior con el contexto default.
//
// Es deliberadamente una capa delgada sobre las propuestas puras (propose.go)
// y el Detector: por eso no tiene tests unitarios propios (requeriría simular
// la TUI de huh) — la lógica con valor de prueba (ProposeX) sí está cubierta.
func Run(ctx context.Context, det Detector, existing *config.Config) (*config.Config, string, error) {
	cfg := existing
	if cfg == nil {
		cfg = &config.Config{}
	}

	for {
		newCtx, err := promptContext(ctx, det)
		if err != nil {
			return nil, "", err
		}

		for {
			if addErr := cfg.AddContext(newCtx); addErr != nil {
				fmt.Println(addErr.Error())
				name, err := askInput("Context name", newCtx.Name, "")
				if err != nil {
					return nil, "", err
				}
				newCtx.Name = name
				continue
			}
			break
		}

		another, err := askConfirm("Add another context?", false)
		if err != nil {
			return nil, "", err
		}
		if !another {
			break
		}
	}

	if err := promptDefaultContext(cfg); err != nil {
		return nil, "", err
	}

	path, err := promptLocation(existing)
	if err != nil {
		return nil, "", err
	}

	return cfg, path, nil
}

// promptContext recorre los pasos de un contexto nuevo: cloud, profile,
// region, cluster, propuestas (nombre/template/writable) e imágenes.
func promptContext(ctx context.Context, det Detector) (config.Context, error) {
	var c config.Context

	cloud, err := promptCloud()
	if err != nil {
		return c, err
	}
	c.Cloud = cloud

	profile, err := promptProfile(det)
	if err != nil {
		return c, err
	}
	c.Profile = profile

	region, err := askInput("Region", "", "leave empty to use the profile's default")
	if err != nil {
		return c, err
	}
	c.Region = region

	cluster, err := promptCluster(ctx, det, profile, region)
	if err != nil {
		return c, err
	}
	c.Cluster = cluster

	name := ProposeName(cluster)
	serviceTemplate := ProposeServiceTemplate(cluster)
	writable := ProposeWritable(name)
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Context name").Value(&name),
		huh.NewInput().Title("Service template").Description("Use {name} as placeholder").Value(&serviceTemplate),
		huh.NewConfirm().Title("Writable (allow deploys from this context)?").Value(&writable),
	))
	if err := form.Run(); err != nil {
		return c, err
	}
	c.Name = name
	c.ServiceTemplate = serviceTemplate
	c.Writable = writable

	images, err := promptImages(serviceTemplate)
	if err != nil {
		return c, err
	}
	c.Images = images

	return c, nil
}

// promptCloud repite el picker hasta que se elige un cloud implementado
// (gcp/azure se muestran para transparencia, pero no tienen provider aún).
func promptCloud() (string, error) {
	for {
		opts := make([]huh.Option[string], 0, len(cloudOptions))
		for _, cl := range cloudOptions {
			label := cl.label
			if !providers.IsImplemented(cl.id) {
				label += " (not implemented yet)"
			}
			opts = append(opts, huh.NewOption(label, cl.id))
		}
		var cloud string
		form := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Cloud").Options(opts...).Value(&cloud),
		))
		if err := form.Run(); err != nil {
			return "", err
		}
		if providers.IsImplemented(cloud) {
			return cloud, nil
		}
		fmt.Printf("%s is not implemented yet — pick a different cloud.\n", cloud)
	}
}

// promptProfile ofrece los perfiles detectados en ~/.aws; sin detección (o
// lista vacía) enseña el remedio y cae a un input manual.
func promptProfile(det Detector) (string, error) {
	profiles, err := det.Profiles()
	if err != nil {
		fmt.Println(providers.Friendly(err))
		return askInput("AWS profile", "", "")
	}
	if len(profiles) == 0 {
		fmt.Println("No AWS profiles found — install the AWS CLI and run: aws configure")
		return askInput("AWS profile", "", "")
	}

	opts := make([]huh.Option[string], 0, len(profiles)+1)
	for _, p := range profiles {
		opts = append(opts, huh.NewOption(p, p))
	}
	opts = append(opts, huh.NewOption("Other (type manually)", ""))

	var profile string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("AWS profile").Options(opts...).Value(&profile),
	))
	if err := form.Run(); err != nil {
		return "", err
	}
	if profile == "" {
		return askInput("AWS profile", "", "")
	}
	return profile, nil
}

// promptCluster ofrece los clusters detectados con ese perfil/región; error
// del Detector enseña el remedio (Friendly) y cae a un input manual.
func promptCluster(ctx context.Context, det Detector, profile, region string) (string, error) {
	clusters, err := det.Clusters(ctx, profile, region)
	if err != nil {
		fmt.Println(providers.Friendly(err))
		return askInput("Cluster", "", "")
	}
	if len(clusters) == 0 {
		fmt.Println("No clusters found for that profile/region.")
		return askInput("Cluster", "", "")
	}

	opts := make([]huh.Option[string], 0, len(clusters)+1)
	for _, c := range clusters {
		opts = append(opts, huh.NewOption(c, c))
	}
	opts = append(opts, huh.NewOption("Other (type manually)", ""))

	var cluster string
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Cluster").Options(opts...).Value(&cluster),
	))
	if err := form.Run(); err != nil {
		return "", err
	}
	if cluster == "" {
		return askInput("Cluster", "", "")
	}
	return cluster, nil
}

// promptImages pregunta si habilitar el registry de imágenes; de ser así,
// precarga repo_template con ProposeRepoTemplate (editable).
func promptImages(serviceTemplate string) (*config.ImagesConfig, error) {
	enable, err := askConfirm("Configure the image registry (enables the deploy tag-picker)?", false)
	if err != nil {
		return nil, err
	}
	if !enable {
		return nil, nil
	}
	repoTemplate, err := askInput("Repo template", ProposeRepoTemplate(serviceTemplate), "Use {name} as placeholder")
	if err != nil {
		return nil, err
	}
	return &config.ImagesConfig{RepoTemplate: repoTemplate}, nil
}

// promptDefaultContext elige el default_context entre los nombres actuales
// cuando hay más de uno (con uno solo, AddContext ya lo fijó).
func promptDefaultContext(cfg *config.Config) error {
	names := make([]string, 0, len(cfg.Contexts))
	for n := range cfg.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) <= 1 {
		return nil
	}

	def := cfg.DefaultContext
	if def == "" {
		def = names[0]
	}
	opts := make([]huh.Option[string], 0, len(names))
	for _, n := range names {
		opts = append(opts, huh.NewOption(n, n))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Default context").Options(opts...).Value(&def),
	))
	if err := form.Run(); err != nil {
		return err
	}
	cfg.DefaultContext = def
	return nil
}

// promptLocation decide dónde vivirá el archivo. En modo add (existing != nil)
// respeta la ubicación del archivo original sin preguntar.
func promptLocation(existing *config.Config) (string, error) {
	if existing != nil {
		return config.Find()
	}

	global, err := config.GlobalPath()
	if err != nil {
		return "", err
	}
	location := global
	opts := []huh.Option[string]{
		huh.NewOption("Global (~/.config/steer/steer.toml)", global),
		huh.NewOption("This repo (./steer.toml)", "steer.toml"),
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("Where should this config live?").Options(opts...).Value(&location),
	))
	if err := form.Run(); err != nil {
		return "", err
	}
	return location, nil
}

// askInput es un input de una sola línea con placeholder opcional.
func askInput(title, value, placeholder string) (string, error) {
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title(title).Placeholder(placeholder).Value(&value),
	))
	if err := form.Run(); err != nil {
		return "", err
	}
	return value, nil
}

// askConfirm es un confirm sí/no con default.
func askConfirm(title string, def bool) (bool, error) {
	value := def
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title(title).Value(&value),
	))
	if err := form.Run(); err != nil {
		return false, err
	}
	return value, nil
}
