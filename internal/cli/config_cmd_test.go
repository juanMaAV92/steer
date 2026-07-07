package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

// minimalToml es un steer.toml válido con un contexto dev usado por los tests que no son de config.
const minimalToml = "[contexts.dev]\ncloud=\"aws\"\nprofile=\"dev\"\ncluster=\"dev-cluster\"\nwritable=true\n"

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	// Non-config commands require a steer.toml; provide a minimal one in a temp
	// dir when none exists and the first arg is not "config".
	needsConfig := len(args) > 0 && args[0] != "config"
	if needsConfig {
		if _, err := os.Stat("steer.toml"); os.IsNotExist(err) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "steer.toml"), []byte(minimalToml), 0o600))
			t.Chdir(dir)
		}
	}
	return runRootCtx(t, context.Background(), args...)
}

// runRootCtx es como runRoot pero permite inyectar un contexto propio, usado
// por tests que verifican propagación de cancelación (ExecuteContext).
func runRootCtx(t *testing.T, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd("test")
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewServiceCmd())
	root.AddCommand(NewImageCmd())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	return out.String(), err
}

// TestConfigInitExampleKeepsLegacyBehavior cubre `config init --example`: el
// dump estático histórico, sin tocar el wizard interactivo (no testeable aquí).
func TestConfigInitExampleKeepsLegacyBehavior(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	out, err := runRootCtx(t, context.Background(), "config", "init", "--example")
	require.NoError(t, err)
	require.Contains(t, out, "created steer.toml")

	raw, err := os.ReadFile(filepath.Join(dir, "steer.toml"))
	require.NoError(t, err)
	require.Contains(t, string(raw), "[contexts.dev]")

	// segunda vez falla como siempre (no sobreescribe).
	_, err = runRootCtx(t, context.Background(), "config", "init", "--example")
	require.ErrorContains(t, err, "already exists")
}

// TestConfigAddWithoutConfigTeaches cubre el camino no interactivo de `config
// add`: sin steer.toml (ni local ni global) debe enseñar el remedio en vez de
// arrancar el wizard.
func TestConfigAddWithoutConfigTeaches(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir()) // sin global tampoco

	_, err := runRootCtx(t, context.Background(), "config", "add")
	require.ErrorContains(t, err, "steer config init")
}

func TestConfigValidateOK(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "steer.toml"),
		[]byte("[contexts.dev]\ncloud=\"aws\"\nprofile=\"dev\"\ncluster=\"dev-cluster\"\nwritable=true\n"), 0o600))

	out, err := runRoot(t, "config", "validate")
	require.NoError(t, err)
	require.Contains(t, out, "1 contexts")
}

func TestConfigValidateFailsWithoutContexts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "steer.toml"), []byte("\n"), 0o600))

	_, err := runRoot(t, "config", "validate")
	require.Error(t, err)
}

// writeTestConfig escribe un steer.toml con dos contextos (dev default, prod read-only).
func writeTestConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "steer.toml")
	require.NoError(t, os.WriteFile(path, []byte(`default_context = "dev"

[contexts.dev]
cloud = "aws"
profile = "dev"
cluster = "dev-cluster"
writable = true

[contexts.prod]
cloud = "aws"
profile = "prod"
cluster = "prod-cluster"
writable = false
`), 0o600))
	return path
}

func TestConfigListShowsContexts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTestConfig(t, dir)
	out, err := runRoot(t, "config", "list")
	require.NoError(t, err)
	require.Contains(t, out, "dev")
	require.Contains(t, out, "prod")
	require.Contains(t, out, "default") // marca cuál es el default
	require.Contains(t, out, "read-only")
}

func TestConfigRemoveDeletesAndReassigns(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := writeTestConfig(t, dir)
	out, err := runRoot(t, "config", "remove", "dev", "-y")
	require.NoError(t, err)
	require.Contains(t, out, "removed")
	require.Contains(t, out, "default_context is now") // avisa la reasignación
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, "prod", cfg.DefaultContext)
	_, err = runRoot(t, "config", "remove", "nope", "-y")
	require.ErrorContains(t, err, "not found")
}
