package cli

import (
	"context"
	"os"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/spf13/cobra"
)

type ctxKey struct{}

// FromContext recupera el AppContext de un cobra.Command.
func FromContext(ctx context.Context) *AppContext {
	if a, ok := ctx.Value(ctxKey{}).(*AppContext); ok {
		return a
	}
	return nil
}

// NewRootCmd construye el comando raíz `steer`.
func NewRootCmd(version string) *cobra.Command {
	var contextName string

	root := &cobra.Command{
		Use:           "steer",
		Short:         "Steer your cloud from the terminal",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&contextName, "context", "", "target context (default: STEER_CONTEXT or default_context)")
	root.PersistentFlags().StringVar(&contextName, "env", "", "alias of --context")
	_ = root.PersistentFlags().MarkDeprecated("env", "use --context instead")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		factory := providers.NewDeployerFactory()
		// Los comandos `config` no requieren un steer.toml ya cargado.
		if cmd.Parent() != nil && cmd.Parent().Name() == "config" {
			cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, &AppContext{Factory: factory}))
			return nil
		}
		path, err := config.Find()
		if err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		if contextName == "" {
			contextName = os.Getenv("STEER_CONTEXT")
		}
		cur, err := cfg.ResolveContext(contextName)
		if err != nil {
			return err
		}
		if err := cur.Validate(); err != nil {
			return err
		}
		app := &AppContext{Ctx: cur, Config: cfg, Factory: factory}
		cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, app))
		return nil
	}

	return root
}
