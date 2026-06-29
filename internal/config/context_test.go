package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTOML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "steer.toml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

const sampleContexts = `
default_context = "nao-dev"

[contexts.nao-dev]
cloud            = "aws"
profile          = "dev"
cluster          = "nao-v2-dev-cluster"
service_template = "nao-v2-dev-{name}"
writable         = true

[contexts.nao-prod]
cloud            = "aws"
profile          = "prod"
cluster          = "nao-v2-production-cluster"
service_template = "nao-v2-production-{name}"
writable         = false
`

func TestLoadParsesContexts(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)

	dev, err := cfg.Context("nao-dev")
	require.NoError(t, err)
	require.Equal(t, "nao-dev", dev.Name)
	require.Equal(t, "aws", dev.Cloud)
	require.Equal(t, "dev", dev.Profile)
	require.Equal(t, "nao-v2-dev-cluster", dev.Cluster)
	require.True(t, dev.Writable)

	prod, err := cfg.Context("nao-prod")
	require.NoError(t, err)
	require.False(t, prod.Writable)
}

func TestContextUnknown(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	_, err = cfg.Context("ghost")
	require.ErrorContains(t, err, "ghost")
}

func TestDefaultCtxUsesDefaultContext(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	d, err := cfg.DefaultCtx()
	require.NoError(t, err)
	require.Equal(t, "nao-dev", d.Name)
}

func TestDefaultCtxSingleContextNoDefault(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
[contexts.only]
cloud   = "aws"
profile = "dev"
cluster = "c"
`))
	require.NoError(t, err)
	d, err := cfg.DefaultCtx()
	require.NoError(t, err)
	require.Equal(t, "only", d.Name)
}

func TestDefaultCtxAmbiguous(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
[contexts.a]
cloud = "aws"
profile = "x"
cluster = "ca"
[contexts.b]
cloud = "aws"
profile = "y"
cluster = "cb"
`))
	require.NoError(t, err)
	_, err = cfg.DefaultCtx()
	require.ErrorContains(t, err, "default_context")
}

func TestAllContextsSorted(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	all := cfg.AllContexts()
	require.Len(t, all, 2)
	require.Equal(t, "nao-dev", all[0].Name)
	require.Equal(t, "nao-prod", all[1].Name)
}

func TestResolveContextFlagOrDefault(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	byFlag, err := cfg.ResolveContext("nao-prod")
	require.NoError(t, err)
	require.Equal(t, "nao-prod", byFlag.Name)
	byDefault, err := cfg.ResolveContext("")
	require.NoError(t, err)
	require.Equal(t, "nao-dev", byDefault.Name)
}

func TestContextServiceNameAndPrefix(t *testing.T) {
	c := Context{ServiceTemplate: "nao-v2-dev-{name}"}
	require.Equal(t, "nao-v2-dev-audit-ms", c.ServiceName("audit-ms"))
	require.Equal(t, "nao-v2-dev-", c.Prefix())

	bare := Context{}
	require.Equal(t, "audit-ms", bare.ServiceName("audit-ms")) // sin template → sin cambio
	require.Equal(t, "", bare.Prefix())
}

func TestContextValidate(t *testing.T) {
	require.NoError(t, Context{Cloud: "aws", Profile: "dev", Cluster: "c"}.Validate())
	require.Error(t, Context{Cloud: "aws", Cluster: "c"}.Validate())   // aws sin profile
	require.Error(t, Context{Profile: "dev", Cluster: "c"}.Validate()) // sin cloud
	require.Error(t, Context{Cloud: "aws", Profile: "d"}.Validate())   // sin cluster
}
