package cli

import (
	"context"
	"fmt"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/tui"
	"github.com/spf13/cobra"
)

// NewTuiCmd construye `steer tui`. Define su propio PersistentPreRunE (en vez
// de heredar el de la raíz) porque necesita envolver el error de "sin config":
// la TUI no puede arrancar sin un steer.toml, y a diferencia de la CLI el
// remedio (el wizard) merece mencionarse explícitamente aquí.
func NewTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive dashboard",
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := config.Find(); err != nil {
				return fmt.Errorf("no steer.toml found — try: steer config init (interactive setup)")
			}
			factory := newProviderFactoryFn()
			contextName, _ := cmd.Flags().GetString("context")
			app, err := buildAppContext(contextName, factory)
			if err != nil {
				return err
			}
			cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, app))
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			return tui.Run(cmd.Context(), app.Factory, app.Config.AllContexts(), app.Ctx)
		},
	}
}
