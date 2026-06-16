package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadParsesEnvironments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "steer.toml")
	content := `
[providers.aws.environments.staging]
profile    = "staging"
account_id = "000000000000"
role_arn   = "arn:aws:iam::000000000000:role/deployer"
writable   = true

[providers.aws.naming]
cluster_template = "{env}-cluster"
service_template = "{name}"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)

	stg := cfg.Providers.AWS.Environments["staging"]
	require.Equal(t, "staging", stg.Profile)
	require.Equal(t, "000000000000", stg.AccountID)
	require.True(t, stg.Writable)
	require.Equal(t, "{env}-cluster", cfg.Providers.AWS.Naming.ClusterTemplate)
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	require.Error(t, err)
}
