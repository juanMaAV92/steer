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
			return tui.Run(app.Factory, app.Config.AllContexts(), app.Ctx)
		},
	}
}
