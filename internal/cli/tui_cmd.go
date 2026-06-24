package cli

import (
	"github.com/juanMaAV92/steer/internal/tui"
	"github.com/spf13/cobra"
)

// NewTuiCmd construye `steer tui`.
func NewTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive dashboard",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			dep, cluster, err := newDeployerFn(app)
			if err != nil {
				return err
			}
			return tui.Run(dep, cluster, app.EnvName)
		},
	}
}
