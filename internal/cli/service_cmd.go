package cli

import (
	"context"
	"fmt"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/providers/aws"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

// newDeployerFn es un seam inyectable: en tests se reemplaza por un fake.
var newDeployerFn = func(app *AppContext) (core.Deployer, string, error) {
	cfg, err := aws.LoadConfig(context.Background(), app.Env)
	if err != nil {
		return nil, "", err
	}
	cluster := app.Config.Providers.AWS.Naming.Cluster(app.EnvName)
	return aws.NewDeployer(cfg), cluster, nil
}

// NewServiceCmd agrupa los comandos de la capacidad service.
func NewServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"svc"},
		Short:   "Manage compute services (deploy, scale, status...)",
	}
	cmd.AddCommand(newServiceStatusCmd())
	return cmd
}

func newServiceStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "status",
		Aliases: []string{"ls"},
		Short:   "List services and their running/desired counts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			dep, cluster, err := newDeployerFn(app)
			if err != nil {
				return err
			}
			services, err := dep.ListServices(cmd.Context(), cluster)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			for _, s := range services {
				fmt.Fprintf(out, "%s %s\t%d/%d\n",
					render.Symbol(render.StatusLevel(s.Running, s.Desired)),
					s.Name, s.Running, s.Desired)
			}
			return nil
		},
	}
	return cmd
}
