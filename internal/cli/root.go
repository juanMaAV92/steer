package cli

import (
	"context"

	"github.com/juanMaAV92/steer/internal/config"
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
	var envName string

	root := &cobra.Command{
		Use:           "steer",
		Short:         "Steer your cloud from the terminal",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&envName, "env", "e", "dev", "target environment")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		// Los comandos `config` no requieren un steer.toml ya cargado.
		if cmd.Parent() != nil && cmd.Parent().Name() == "config" {
			cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, &AppContext{EnvName: envName}))
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
		env, err := cfg.Env(envName)
		if err != nil {
			return err
		}
		app := &AppContext{EnvName: envName, Env: env, Config: cfg}
		cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, app))
		return nil
	}

	return root
}
