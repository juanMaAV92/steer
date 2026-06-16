package cli

import (
	"fmt"
	"os"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/spf13/cobra"
)

const exampleConfig = `[providers.aws.environments.dev]
profile  = "dev"
writable = true

[providers.aws.environments.prod]
profile  = "prod"
writable = false

[providers.aws.naming]
cluster_template = "{env}-cluster"
service_template = "{name}"
`

// NewConfigCmd agrupa `steer config init|validate`.
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage steer configuration"}
	cmd.AddCommand(newConfigInitCmd(), newConfigValidateCmd())
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a starter steer.toml in the current directory",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if _, err := os.Stat("steer.toml"); err == nil {
				return fmt.Errorf("steer.toml already exists")
			}
			if err := os.WriteFile("steer.toml", []byte(exampleConfig), 0o600); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "created steer.toml")
			return nil
		},
	}
}

func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the discovered steer.toml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Find()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if len(cfg.Providers.AWS.Environments) == 0 {
				return fmt.Errorf("config %s has no environments", path)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok: %s (%d environments)\n",
				path, len(cfg.Providers.AWS.Environments))
			return nil
		},
	}
}
