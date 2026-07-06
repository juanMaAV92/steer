package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

// NewServiceCmd agrupa los comandos de la capacidad service.
func NewServiceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "service",
		Aliases: []string{"svc"},
		Short:   "Manage compute services (deploy, scale, status...)",
	}
	cmd.AddCommand(newServiceStatusCmd(), newServiceDeployCmd(), newServiceScaleCmd(), newServiceRollbackCmd(), newServiceResizeCmd())
	return cmd
}

// serviceStatusTable construye la tabla de estado de servicios.
func serviceStatusTable(services []core.ServiceStatus) string {
	headers := []string{"", "SERVICE", "DESIRED", "RUNNING", "PENDING", "STATUS", "TAG", "CPU", "MEM"}
	rows := make([][]string, 0, len(services))
	for _, s := range services {
		running := strconv.Itoa(s.Running)
		if s.Running != s.Desired { // no alcanza el deseado → rojo
			running = render.Danger(running)
		}
		pending := strconv.Itoa(s.Pending)
		if s.Pending > 0 { // hay instancias arrancando → amarillo
			pending = render.Warn(pending)
		}
		cpu, mem := "—", "—"
		if s.Resources != (core.Resources{}) {
			cpu = render.CPULabel(s.Resources.CPUMilli)
			mem = render.MemLabel(s.Resources.MemoryMiB)
		}
		rows = append(rows, []string{
			render.Symbol(render.StatusLevel(s.Running, s.Desired)),
			s.Name,
			strconv.Itoa(s.Desired),
			running,
			pending,
			s.Status,
			render.Accent(s.Tag),
			cpu,
			mem,
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
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			render1 := func() (string, error) {
				services, err := dep.ListServices(cmd.Context())
				if err != nil {
					return "", err
				}
				return serviceStatusTable(services) + "\n", nil
			}
			if !watch {
				content, err := render1()
				if err != nil {
					return err
				}
				_, _ = fmt.Fprint(out, content)
				return nil
			}
			// --watch: redibuja en el sitio subiendo el cursor las líneas previas.
			header := fmt.Sprintf("%s  %s  %s\n", render.Bold("steer"), render.Dim(app.Ctx.Cluster),
				render.Dim(fmt.Sprintf("(refresh %ds, Ctrl+C to stop)", interval)))
			prevLines := 0
			for {
				content, err := render1()
				if err != nil {
					return err
				}
				frame := header + content
				if prevLines > 0 {
					_, _ = fmt.Fprintf(out, "\033[%dA\033[J", prevLines) // sube N líneas y limpia hacia abajo
				}
				_, _ = fmt.Fprint(out, frame)
				prevLines = strings.Count(frame, "\n")
				select {
				case <-cmd.Context().Done():
					_, _ = fmt.Fprintln(out)
					return cmd.Context().Err()
				case <-time.After(time.Duration(interval) * time.Second):
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "refresh continuously")
	cmd.Flags().IntVar(&interval, "interval", 15, "refresh interval in seconds for --watch")
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
			if yes && (service == "" || tag == "") {
				return fmt.Errorf("non-interactive deploy (-y) requires --service and --tag")
			}
			if err := app.RequireWritable(); err != nil {
				return err
			}
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}

			if service == "" || tag == "" {
				services, err := dep.ListServices(cmd.Context())
				if err != nil {
					return err
				}
				s, tg, ok, err := pickServiceAndTag(serviceOptions(services))
				if err != nil {
					return err
				}
				if !ok {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "aborted")
					return nil
				}
				service, tag = s, tg
			}
			realName := app.Ctx.ServiceName(service)

			// validación del tag contra el registry: estricta si no existe,
			// degradable si el registry no está disponible (nunca bloquea CI).
			if reg, rerr := app.Registry(cmd.Context()); rerr == nil {
				repo := app.Ctx.RepoName(service)
				switch found, herr := reg.HasTag(cmd.Context(), repo, tag); {
				case errors.Is(herr, core.ErrRepoNotFound):
					return fmt.Errorf("repository %q not found (check images.repo_template)", repo)
				case herr != nil:
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
						render.Warn("warning: registry check skipped: "+herr.Error()))
				case !found:
					return fmt.Errorf("tag %q not found in repository %q", tag, repo)
				}
			} else {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
					render.Warn("warning: registry check skipped (images not configured or registry unavailable)"))
			}

			current, err := dep.CurrentTag(cmd.Context(), realName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%s (%s):\n  %s: %s %s %s\n",
				render.Bold("Deploy preview"), app.Ctx.Name,
				render.Bold(service), render.Dim(current), render.Dim("->"), render.Accent(tag))

			if !yes {
				_, _ = fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, render.Dim("aborted"))
					return nil
				}
			}

			if err := dep.Deploy(cmd.Context(), realName, tag, func(s string) {
				_, _ = fmt.Fprintln(out, render.Dim("[*] "+s))
			}); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s %s %s %s\n%s\n",
				render.Success("✓ deployed"), render.Bold(service), render.Dim("->"), render.Accent(tag),
				render.Dim(fmt.Sprintf("rollback with: steer --context %s service rollback -s %s", app.Ctx.Name, service)))

			if watch {
				return watchRollout(cmd.Context(), out, dep, realName, service, interval)
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

// watchRollout sigue el rollout: streaming de eventos + línea de status, hasta
// COMPLETED/FAILED o hasta detectar un atasco (3 fallos de aprovisionamiento).
func watchRollout(ctx context.Context, out io.Writer, dep core.Deployer, service, short string, interval int) error {
	_, _ = fmt.Fprintln(out, render.Dim("monitoring rollout (Ctrl+C to stop)..."))

	// Marca el último evento ya existente para solo mostrar los nuevos.
	lastID := ""
	if evs, err := dep.ServiceEvents(ctx, service); err == nil && len(evs) > 0 {
		lastID = evs[0].ID
	}

	// La línea de status se mantiene SIEMPRE como última línea: se borra, se
	// imprimen los eventos nuevos encima (se acumulan) y se reescribe abajo.
	statusShown := false
	pullErrors := 0
	for {
		if statusShown {
			_, _ = fmt.Fprint(out, "\r\033[K") // borra la línea de status actual
		}

		// Eventos nuevos (ECS los entrega más recientes primero) → se acumulan.
		if evs, err := dep.ServiceEvents(ctx, service); err == nil {
			var fresh []core.ServiceEvent
			for _, e := range evs {
				if e.ID == lastID {
					break
				}
				fresh = append(fresh, e)
			}
			if len(fresh) > 0 {
				lastID = fresh[0].ID
			}
			for i := len(fresh) - 1; i >= 0; i-- { // del más antiguo al más nuevo
				printEvent(out, fresh[i])
				if core.IsProvisioningFailure(fresh[i].Message) {
					pullErrors++
				}
			}
		}

		d, err := dep.DeploymentStatus(ctx, service)
		if err != nil {
			_, _ = fmt.Fprintln(out)
			return err
		}
		// status sin salto de línea: queda como última línea, lista para reescribir.
		_, _ = fmt.Fprintf(out, "Rollout: %s | Running: %d | Pending: %d | Desired: %d",
			render.Rollout(string(d.Rollout)), d.Running, d.Pending, d.Desired)
		statusShown = true

		if d.Rollout == core.RolloutCompleted && d.Running >= d.Desired {
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, render.Success("✓ deployment completed"))
			return nil
		}
		if d.Rollout == core.RolloutFailed {
			_, _ = fmt.Fprintln(out)
			return fmt.Errorf("deployment failed for %q", service)
		}
		// 3 fallos de aprovisionamiento = rollout atascado (ECS reintenta para
		// siempre sin circuit breaker y nunca reporta FAILED): cortar el poll.
		// La completación/fallo reportado por ECS gana sobre la heurística.
		if pullErrors >= 3 {
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, render.Danger("✗ deployment stuck: image pull failing"))
			_, _ = fmt.Fprintln(out, render.Dim("roll back with: steer service rollback -s "+short))
			return fmt.Errorf("deployment stuck for %q: image pull keeps failing", service)
		}
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintln(out)
			return ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
	}
}

// printEvent imprime un evento de servicio con timestamp; rojo si es error, gris si no.
func printEvent(out io.Writer, e core.ServiceEvent) {
	line := fmt.Sprintf("[%s] %s", e.At.Format("15:04:05"), e.Message)
	if e.IsError {
		_, _ = fmt.Fprintln(out, render.Danger(line))
		return
	}
	_, _ = fmt.Fprintln(out, render.Dim(line))
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
			if !cmd.Flags().Changed("count") {
				return fmt.Errorf("--count is required (refusing to default to 1)")
			}
			app := FromContext(cmd.Context())
			if err := app.RequireWritable(); err != nil {
				return err
			}
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}
			realName := app.Ctx.ServiceName(service)
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Scale %s to %d in %s\n", service, count, app.Ctx.Name)
			if !yes {
				_, _ = fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, "aborted")
					return nil
				}
			}
			if err := dep.Scale(cmd.Context(), realName, count); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s %s %s\n", render.Success("✓ scaled"), render.Bold(service), render.Dim(fmt.Sprintf("to %d", count)))
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().IntVarP(&count, "count", "c", 0, "desired task count (required)")
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
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}
			realName := app.Ctx.ServiceName(service)
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "Roll back %s in %s to previous revision\n", service, app.Ctx.Name)
			if !yes {
				_, _ = fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, "aborted")
					return nil
				}
			}
			if err := dep.Rollback(cmd.Context(), realName); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s %s\n", render.Success("✓ rolled back"), render.Bold(service))
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
}

func newServiceResizeCmd() *cobra.Command {
	var service, cpu, memory string
	var yes, watch bool
	var interval int
	cmd := &cobra.Command{
		Use:   "resize",
		Short: "Update the CPU/memory of a service (new revision + rollout)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" || cpu == "" || memory == "" {
				return fmt.Errorf("--service, --cpu and --memory are required")
			}
			app := FromContext(cmd.Context())
			if err := app.RequireWritable(); err != nil {
				return err
			}
			cpuMilli, err := parseCPU(cpu)
			if err != nil {
				return err
			}
			memMiB, err := parseMemory(memory)
			if err != nil {
				return err
			}
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}
			// validación que enseña, derivada de la tabla del provider
			opts := dep.ResourceOptions()
			var tier *core.ResourceOption
			for i := range opts {
				if opts[i].CPUMilli == cpuMilli {
					tier = &opts[i]
					break
				}
			}
			if tier == nil {
				var tiers []string
				for _, o := range opts {
					tiers = append(tiers, render.CPULabel(o.CPUMilli))
				}
				return fmt.Errorf("valid cpu tiers: %s", strings.Join(tiers, ", "))
			}
			validMem := false
			for _, m := range tier.MemoryMiB {
				if m == memMiB {
					validMem = true
					break
				}
			}
			if !validMem {
				var mems []string
				for _, m := range tier.MemoryMiB {
					mems = append(mems, render.MemLabel(m))
				}
				return fmt.Errorf("cpu %s supports: %s; got %s",
					render.CPULabel(cpuMilli), strings.Join(mems, ", "), render.MemLabel(memMiB))
			}
			realName := app.Ctx.ServiceName(service)
			// recursos actuales para el preview (mejor esfuerzo)
			currentLabel := "unknown"
			if svcs, err := dep.ListServices(cmd.Context()); err == nil {
				for _, s := range svcs {
					if s.Name == realName && s.Resources != (core.Resources{}) {
						currentLabel = render.CPULabel(s.Resources.CPUMilli) + " · " + render.MemLabel(s.Resources.MemoryMiB)
					}
				}
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%s (%s):\n  %s: %s %s %s\n",
				render.Bold("Resize preview"), app.Ctx.Name, render.Bold(service),
				render.Dim(currentLabel), render.Dim("->"),
				render.Accent(render.CPULabel(cpuMilli)+" · "+render.MemLabel(memMiB)))
			if !yes {
				_, _ = fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, render.Dim("aborted"))
					return nil
				}
			}
			if err := dep.Resize(cmd.Context(), realName, core.Resources{CPUMilli: cpuMilli, MemoryMiB: memMiB},
				func(s string) { _, _ = fmt.Fprintln(out, render.Dim("[*] "+s)) }); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s %s\n%s\n",
				render.Success("✓ resized"), render.Bold(service),
				render.Dim(fmt.Sprintf("rollback with: steer --context %s service rollback -s %s", app.Ctx.Name, service)))
			if watch {
				return watchRollout(cmd.Context(), out, dep, realName, service, interval)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().StringVar(&cpu, "cpu", "", "target cpu (0.5, 1 or 500m)")
	cmd.Flags().StringVar(&memory, "memory", "", "target memory (2048, 2GB or 512MB)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "follow the rollout until it completes")
	cmd.Flags().IntVar(&interval, "interval", 3, "poll interval in seconds for --watch")
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
