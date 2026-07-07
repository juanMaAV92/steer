package cli

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/juanMaAV92/steer/internal/cli/wizard"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/juanMaAV92/steer/internal/providers/aws"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

const exampleConfig = `default_context = "dev"

[contexts.dev]
cloud            = "aws"
profile          = "dev"
cluster          = "dev-cluster"
service_template = "{name}"
writable         = true

[contexts.prod]
cloud            = "aws"
profile          = "prod"
cluster          = "prod-cluster"
service_template = "{name}"
writable         = false
`

// newWizardDetector es un seam de construcción (no de comportamiento): hoy
// solo AWS está implementado (providers.IsImplemented), pero el wizard es
// agnóstico de cloud vía wizard.Detector.
var newWizardDetector = func() wizard.Detector { return aws.NewDetector() }

// aws.Detector debe satisfacer wizard.Detector. La aserción vive aquí (no en el
// paquete wizard) para que el flujo agnóstico no importe ningún provider concreto:
// cada cloud nuevo añade su aserción en su punto de inyección, no en el wizard.
var _ wizard.Detector = (*aws.Detector)(nil)

// NewConfigCmd agrupa `steer config init|add|remove|list|validate`.
func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage steer configuration"}
	cmd.AddCommand(newConfigInitCmd(), newConfigAddCmd(), newConfigRemoveCmd(), newConfigListCmd(), newConfigValidateCmd())
	return cmd
}

func newConfigInitCmd() *cobra.Command {
	var example bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a steer.toml (interactive wizard) or a starter example",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if example {
				return runExampleInit(cmd)
			}
			path, err := config.Find()
			if err != nil {
				// sin config previa: wizard desde cero.
				return runWizardAndWrite(cmd, nil)
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			return runInitWithExisting(cmd, path, cfg)
		},
	}
	cmd.Flags().BoolVar(&example, "example", false, "write the static example config (non-interactive, legacy)")
	return cmd
}

// runExampleInit reproduce el comportamiento estático histórico de `config init`.
func runExampleInit(cmd *cobra.Command) error {
	if _, err := os.Stat("steer.toml"); err == nil {
		return fmt.Errorf("steer.toml already exists")
	}
	if err := os.WriteFile("steer.toml", []byte(exampleConfig), 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "created steer.toml")
	return nil
}

// runInitWithExisting maneja `config init` cuando ya hay un steer.toml: ofrece
// agregar un contexto, recrear desde cero (destructivo, con confirmación) o
// cancelar.
func runInitWithExisting(cmd *cobra.Command, path string, cfg *config.Config) error {
	all := cfg.AllContexts()
	names := make([]string, 0, len(all))
	for _, c := range all {
		names = append(names, c.Name)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "found %s (%d contexts: %v)\n", path, len(names), names)

	const (
		optAdd      = "add"
		optRecreate = "recreate"
		optCancel   = "cancel"
	)
	choice := optAdd
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().Title("A config already exists — what do you want to do?").
			Options(
				huh.NewOption("Add a context", optAdd),
				huh.NewOption("Recreate from scratch", optRecreate),
				huh.NewOption("Cancel", optCancel),
			).Value(&choice),
	))
	if err := form.Run(); err != nil {
		return err
	}

	switch choice {
	case optAdd:
		return runWizardAndWrite(cmd, cfg)
	case optRecreate:
		confirmed := false
		confirmForm := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("This discards every context in %s. Continue?", path)).
				Value(&confirmed),
		))
		if err := confirmForm.Run(); err != nil {
			return err
		}
		if !confirmed {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "cancelled")
			return nil
		}
		return runWizardAndWrite(cmd, nil)
	default:
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "cancelled")
		return nil
	}
}

func newConfigAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add",
		Short: "Add a context to the existing steer.toml (interactive wizard)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Find()
			if err != nil {
				return fmt.Errorf("no steer.toml found — try: steer config init")
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			return runWizardAndWrite(cmd, cfg)
		},
	}
}

// runWizardAndWrite corre el wizard, escribe el resultado y hace el smoke
// test del contexto default, imprimiendo el resultado (la config ya quedó
// escrita en disco en ese punto, se avisa explícitamente en el error).
func runWizardAndWrite(cmd *cobra.Command, existing *config.Config) error {
	det := newWizardDetector()
	cfg, path, err := wizard.Run(cmd.Context(), det, existing)
	if err != nil {
		return err
	}
	if err := cfg.Write(path); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)

	defCtx, err := cfg.DefaultCtx()
	if err != nil {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), providers.Friendly(err))
		return nil
	}
	n, err := det.SmokeTest(cmd.Context(), defCtx)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "config saved, but the smoke test failed: %s\n", providers.Friendly(err))
		return nil
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "✓ connected — %d services in %s. Try: steer tui\n", n, defCtx.Cluster)
	return nil
}

// newConfigListCmd construye `config list`: tabla NAME/CLOUD/CLUSTER/MODE/DEFAULT.
func newConfigListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the contexts in the discovered steer.toml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Find()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			headers := []string{"NAME", "CLOUD", "CLUSTER", "MODE", "DEFAULT"}
			all := cfg.AllContexts()
			rows := make([][]string, 0, len(all))
			for _, ctx := range all {
				mode := "read-only"
				if ctx.Writable {
					mode = "writable"
				}
				def := ""
				if ctx.Name == cfg.DefaultContext {
					def = "default"
				}
				rows = append(rows, []string{ctx.Name, ctx.Cloud, ctx.Cluster, mode, def})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), render.Table(headers, rows))
			return nil
		},
	}
}

// newConfigRemoveCmd construye `config remove <name>`: confirmación salvo -y,
// borra el contexto y avisa si eso reasignó (o vació) default_context.
func newConfigRemoveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a context from the discovered steer.toml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			path, err := config.Find()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if _, ok := cfg.Contexts[name]; !ok {
				return fmt.Errorf("context %q not found", name)
			}
			out := cmd.OutOrStdout()
			if !yes {
				_, _ = fmt.Fprintf(out, "Are you sure? This removes %q from %s [y/N]: ", name, path)
				if !confirm(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, "aborted")
					return nil
				}
			}
			wasDefault, err := cfg.RemoveContext(name)
			if err != nil {
				return err
			}
			if err := cfg.Write(path); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "removed %q from %s\n", name, path)
			if wasDefault {
				if cfg.DefaultContext == "" {
					_, _ = fmt.Fprintln(out, "no contexts left; default_context is now empty")
				} else {
					_, _ = fmt.Fprintf(out, "default_context is now %q\n", cfg.DefaultContext)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cmd
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
			if err := cfg.Validate(); err != nil {
				return err
			}
			all := cfg.AllContexts()
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "ok: %s (%d contexts)\n", path, len(all))
			return nil
		},
	}
}
