package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

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

// serviceStatusTable construye la tabla de estado de servicios.
func serviceStatusTable(services []core.ServiceStatus) string {
	headers := []string{"", "SERVICE", "DESIRED", "RUNNING", "PENDING", "STATUS", "TAG"}
	rows := make([][]string, 0, len(services))
	for _, s := range services {
		rows = append(rows, []string{
			render.Symbol(render.StatusLevel(s.Running, s.Desired)),
			s.Name,
			strconv.Itoa(s.Desired),
			strconv.Itoa(s.Running),
			strconv.Itoa(s.Pending),
			s.Status,
			render.Accent(s.Tag),
		})
	}
	return render.Table(headers, rows)
}

func newServiceStatusCmd() *cobra.Command {
	var watch bool
	var interval int
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
			out := cmd.OutOrStdout()
			showOnce := func() error {
				services, err := dep.ListServices(cmd.Context(), cluster)
				if err != nil {
					return err
				}
				fmt.Fprintln(out, serviceStatusTable(services))
				return nil
			}
			if !watch {
				return showOnce()
			}
			for {
				fmt.Fprint(out, "\033[H\033[2J") // limpia la pantalla
				fmt.Fprintf(out, "%s  %s  (refresh %ds, Ctrl+C para salir)\n",
					render.Bold("steer"), render.Dim(cluster), interval)
				if err := showOnce(); err != nil {
					return err
				}
				time.Sleep(time.Duration(interval) * time.Second)
			}
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "refresh continuously")
	cmd.Flags().IntVar(&interval, "interval", 3, "refresh interval in seconds for --watch")
	return cmd
}

func newServiceDeployCmd() *cobra.Command {
	var service, tag string
	var yes, watch bool
	var interval int
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
			realName := app.Config.Providers.AWS.Naming.Service(app.EnvName, service)

			current, err := dep.CurrentTag(cmd.Context(), cluster, realName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s):\n  %s: %s %s %s\n",
				render.Bold("Deploy preview"), app.EnvName,
				render.Bold(service), render.Dim(current), render.Dim("->"), render.Accent(tag))

			if !yes {
				fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					fmt.Fprintln(out, render.Dim("aborted"))
					return nil
				}
			}

			if err := dep.Deploy(cmd.Context(), cluster, realName, tag, func(s string) {
				fmt.Fprintln(out, render.Dim("[*] "+s))
			}); err != nil {
				return err
			}
			fmt.Fprintf(out, "%s %s %s %s\n%s\n",
				render.Success("✓ deployed"), render.Bold(service), render.Dim("->"), render.Accent(tag),
				render.Dim(fmt.Sprintf("rollback with: steer -e %s service rollback -s %s", app.EnvName, service)))

			if watch {
				return watchRollout(cmd.Context(), out, dep, cluster, realName, interval)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "image tag to deploy")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "follow the rollout until it completes")
	cmd.Flags().IntVar(&interval, "interval", 3, "poll interval in seconds for --watch")
	return cmd
}

// watchRollout sigue el rollout de un servicio hasta COMPLETED o FAILED,
// actualizando una sola línea en el sitio (no la reimprime).
func watchRollout(ctx context.Context, out io.Writer, dep core.Deployer, cluster, service string, interval int) error {
	fmt.Fprintln(out, render.Dim("monitoring rollout (Ctrl+C to stop)..."))
	for {
		d, err := dep.DeploymentStatus(ctx, cluster, service)
		if err != nil {
			fmt.Fprintln(out)
			return err
		}
		// \r vuelve al inicio, \033[K limpia hasta fin de línea → reescribe en el sitio.
		fmt.Fprintf(out, "\r\033[KRollout: %s | Running: %d | Pending: %d | Desired: %d",
			rolloutColor(d.Rollout), d.Running, d.Pending, d.Desired)
		switch d.Rollout {
		case "COMPLETED":
			fmt.Fprintln(out)
			fmt.Fprintln(out, render.Success("✓ rollout completed"))
			return nil
		case "FAILED":
			fmt.Fprintln(out)
			return fmt.Errorf("rollout failed for %q", service)
		}
		time.Sleep(time.Duration(interval) * time.Second)
	}
}

func rolloutColor(state string) string {
	switch state {
	case "COMPLETED":
		return render.Success(state)
	case "FAILED":
		return render.Danger(state)
	default:
		return render.Accent(state)
	}
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
			realName := app.Config.Providers.AWS.Naming.Service(app.EnvName, service)
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
			fmt.Fprintf(out, "%s %s %s\n", render.Success("✓ scaled"), render.Bold(service), render.Dim(fmt.Sprintf("to %d", count)))
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
			realName := app.Config.Providers.AWS.Naming.Service(app.EnvName, service)
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
			fmt.Fprintf(out, "%s %s\n", render.Success("✓ rolled back"), render.Bold(service))
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
