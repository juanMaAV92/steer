package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	require.Error(t, err)
}

func TestValidateDefaultContextMustExist(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
default_context = "ghost"
[contexts.dev]
cloud = "aws"
profile = "p"
cluster = "c"
`))
	require.NoError(t, err)
	require.ErrorContains(t, cfg.Validate(), "ghost")
}

func TestValidateDetectsLegacySchema(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
[providers.aws.environments.dev]
profile = "dev"
`))
	require.NoError(t, err)
	err = cfg.Validate()
	require.ErrorContains(t, err, "contexts")
}

func TestValidateOK(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
}
