package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

func newServiceLogsCmd() *cobra.Command {
	var service string
	var follow bool
	var lines, interval int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent service logs (all containers, merged)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" {
				return fmt.Errorf("--service is required")
			}
			app := FromContext(cmd.Context())
			src, err := app.Logs(cmd.Context())
			if err != nil {
				return err
			}
			realName := app.Ctx.ServiceName(service)
			out := cmd.OutOrStdout()

			page, err := src.TailLogs(cmd.Context(), realName, lines)
			if err != nil {
				return err
			}
			if len(page.Lines) == 0 {
				_, _ = fmt.Fprintln(out, render.Dim("no logs in the last hour"))
			}
			for _, l := range page.Lines {
				printLogLine(out, l)
			}
			if !follow {
				return nil
			}
			cursor := page.Cursor
			_, _ = fmt.Fprintln(out, render.Dim("following logs (Ctrl+C to stop)..."))
			for {
				select {
				case <-cmd.Context().Done():
					return cmd.Context().Err()
				case <-time.After(time.Duration(interval) * time.Second):
				}
				page, err := src.FollowLogs(cmd.Context(), realName, cursor)
				if err != nil {
					return err
				}
				for _, l := range page.Lines {
					printLogLine(out, l)
				}
				cursor = page.Cursor
			}
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep streaming new lines")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "how many recent lines to show")
	cmd.Flags().IntVar(&interval, "interval", 3, "poll interval in seconds for --follow")
	return cmd
}

// printLogLine imprime una línea de log: HH:MM:SS  [container]  mensaje
// (el contenedor solo aparece cuando la task tiene más de uno).
func printLogLine(out io.Writer, l core.LogLine) {
	ts := render.Dim(l.At.Format("15:04:05"))
	if l.Container != "" {
		_, _ = fmt.Fprintf(out, "%s  %s  %s\n", ts, render.Accent("["+l.Container+"]"), l.Message)
		return
	}
	_, _ = fmt.Fprintf(out, "%s  %s\n", ts, l.Message)
}
