package cli

import (
	"fmt"

	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

// maxEventsShown acota la tabla de events: es una vista de un vistazo,
// no una herramienta de arqueología.
const maxEventsShown = 20

func newServiceEventsCmd() *cobra.Command {
	var service string
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Show recent service events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" {
				return fmt.Errorf("--service is required")
			}
			app := FromContext(cmd.Context())
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}
			realName := app.Ctx.ServiceName(service)
			evs, err := dep.ServiceEvents(cmd.Context(), realName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(evs) == 0 {
				_, _ = fmt.Fprintln(out, render.Dim("no events"))
				return nil
			}
			if len(evs) > maxEventsShown {
				evs = evs[:maxEventsShown] // ServiceEvents entrega más recientes primero
			}
			headers := []string{"TIME", "MESSAGE"}
			rows := make([][]string, 0, len(evs))
			for i := len(evs) - 1; i >= 0; i-- { // ascendente: lo más nuevo abajo
				e := evs[i]
				msg := e.Message
				if e.IsError {
					msg = render.Danger(msg)
				}
				rows = append(rows, []string{render.Dim(e.At.Format("Jan 02 15:04:05")), msg})
			}
			_, _ = fmt.Fprint(out, render.Table(headers, rows))
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	return cmd
}
