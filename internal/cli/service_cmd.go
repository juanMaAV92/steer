package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

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
	cmd.AddCommand(newServiceStatusCmd(), newServiceDeployCmd(), newServiceScaleCmd(), newServiceRollbackCmd())
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

func newServiceDeployCmd() *cobra.Command {
	var service, tag string
	var yes bool
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy a new image tag to a service (preview before applying)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			if err := app.RequireWritable(); err != nil {
				return err
			}
			dep, cluster, err := newDeployerFn(app)
			if err != nil {
				return err
			}

			if service == "" || tag == "" {
				services, err := dep.ListServices(cmd.Context(), cluster)
				if err != nil {
					return err
				}
				s, tg, ok, err := pickServiceAndTag(serviceOptions(services))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
				service, tag = s, tg
			}
			realName := app.Config.Providers.AWS.Naming.Service(service)

			current, err := dep.CurrentTag(cmd.Context(), cluster, realName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Deploy preview (%s):\n  %s: %s -> %s\n",
				app.EnvName, service, current, tag)

			if !yes {
				fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintln(out, "aborted")
					return nil
				}
			}

			if err := dep.Deploy(cmd.Context(), cluster, realName, tag); err != nil {
				return err
			}
			fmt.Fprintf(out, "deployed %s -> %s\nrollback with: steer -e %s service rollback -s %s\n",
				service, tag, app.EnvName, service)
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "image tag to deploy")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newServiceScaleCmd() *cobra.Command {
	var service string
	var count int
	var yes bool
	cmd := &cobra.Command{
		Use:   "scale",
		Short: "Set the desired task count of a service",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" {
				return fmt.Errorf("--service is required")
			}
			app := FromContext(cmd.Context())
			if err := app.RequireWritable(); err != nil {
				return err
			}
			dep, cluster, err := newDeployerFn(app)
			if err != nil {
				return err
			}
			realName := app.Config.Providers.AWS.Naming.Service(service)
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Scale %s to %d in %s\n", service, count, app.EnvName)
			if !yes {
				fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintln(out, "aborted")
					return nil
				}
			}
			if err := dep.Scale(cmd.Context(), cluster, realName, count); err != nil {
				return err
			}
			fmt.Fprintf(out, "scaled %s to %d\n", service, count)
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().IntVarP(&count, "count", "c", 1, "desired task count")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newServiceRollbackCmd() *cobra.Command {
	var service string
	var yes bool
	cmd := &cobra.Command{
		Use:   "rollback",
		Short: "Roll a service back to its previous task definition",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" {
				return fmt.Errorf("--service is required")
			}
			app := FromContext(cmd.Context())
			if err := app.RequireWritable(); err != nil {
				return err
			}
			dep, cluster, err := newDeployerFn(app)
			if err != nil {
				return err
			}
			realName := app.Config.Providers.AWS.Naming.Service(service)
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Roll back %s in %s to previous revision\n", service, app.EnvName)
			if !yes {
				fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintln(out, "aborted")
					return nil
				}
			}
			if err := dep.Rollback(cmd.Context(), cluster, realName); err != nil {
				return err
			}
			fmt.Fprintf(out, "rolled back %s\n", service)
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

// confirm lee una línea de r y devuelve true si es afirmativa (y/yes).
func confirm(r io.Reader) bool {
	line, _ := bufio.NewReader(r).ReadString('\n')
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
