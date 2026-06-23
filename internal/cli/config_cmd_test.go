package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// minimalToml is a valid steer.toml with a dev environment used by non-config tests.
const minimalToml = "[providers.aws.environments.dev]\nprofile=\"dev\"\nwritable=true\n"

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
	root := NewRootCmd("test")
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewServiceCmd())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestConfigInitCreatesFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // Go 1.24+: cambia el cwd al tempdir

	_, err := runRoot(t, "config", "init")
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dir, "steer.toml"))
}

func TestConfigValidateOK(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "steer.toml"),
		[]byte("[providers.aws.environments.dev]\nprofile=\"dev\"\nwritable=true\n"), 0o600))

	_, err := runRoot(t, "config", "validate")
	require.NoError(t, err)
}

func TestConfigValidateFailsWithoutEnvironments(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "steer.toml"), []byte("\n"), 0o600))

	_, err := runRoot(t, "config", "validate")
	require.Error(t, err)
}
