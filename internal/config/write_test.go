package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func devCtx(name string) Context {
	return Context{Name: name, Cloud: "aws", Profile: name, Cluster: name + "-cluster",
		ServiceTemplate: name + "-{name}", Writable: true}
}

func TestAddContextSetsFirstAsDefault(t *testing.T) {
	c := &Config{Contexts: map[string]Context{}}
	require.NoError(t, c.AddContext(devCtx("dev")))
	require.Equal(t, "dev", c.DefaultContext)
	require.NoError(t, c.AddContext(devCtx("stg")))
	require.Equal(t, "dev", c.DefaultContext) // el default no cambia al agregar más
	// duplicado y contexto inválido fallan
	require.ErrorContains(t, c.AddContext(devCtx("dev")), "already exists")
	bad := devCtx("x")
	bad.Cluster = ""
	require.ErrorContains(t, c.AddContext(bad), "missing cluster")
}

func TestRemoveContextReassignsDefault(t *testing.T) {
	c := &Config{Contexts: map[string]Context{}}
	require.NoError(t, c.AddContext(devCtx("dev")))
	require.NoError(t, c.AddContext(devCtx("prod")))
	wasDefault, err := c.RemoveContext("dev")
	require.NoError(t, err)
	require.True(t, wasDefault)
	require.Equal(t, "prod", c.DefaultContext) // reasignado al primero alfabético
	_, err = c.RemoveContext("nope")
	require.ErrorContains(t, err, "not found")
	wasDefault, err = c.RemoveContext("prod")
	require.NoError(t, err)
	require.True(t, wasDefault)
	require.Empty(t, c.DefaultContext) // no queda ninguno
}

func TestWriteRoundTripPreservesEverything(t *testing.T) {
	c := &Config{Contexts: map[string]Context{}}
	dev := devCtx("dev")
	dev.Images = &ImagesConfig{RepoTemplate: "shared-{name}"}
	prod := devCtx("prod")
	prod.Writable = false
	prod.RoleARN = "arn:aws:iam::1:role/deployer"
	prod.Region = "us-east-1"
	require.NoError(t, c.AddContext(dev))
	require.NoError(t, c.AddContext(prod))

	path := filepath.Join(t.TempDir(), "sub", "steer.toml")
	require.NoError(t, c.Write(path))

	// permisos y determinismo
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	raw1, _ := os.ReadFile(path)
	require.NoError(t, c.Write(path))
	raw2, _ := os.ReadFile(path)
	require.Equal(t, string(raw1), string(raw2), "la serialización debe ser determinista")
	require.True(t, strings.Index(string(raw1), "[contexts.dev]") <
		strings.Index(string(raw1), "[contexts.prod]"), "contextos en orden alfabético")

	// round-trip por el loader real
	loaded, err := Load(path)
	require.NoError(t, err)
	require.NoError(t, loaded.Validate())
	require.Equal(t, "dev", loaded.DefaultContext)
	lDev, _ := loaded.Context("dev")
	require.Equal(t, "shared-{name}", lDev.Images.RepoTemplate)
	lProd, _ := loaded.Context("prod")
	require.False(t, lProd.Writable)
	require.Equal(t, "arn:aws:iam::1:role/deployer", lProd.RoleARN)
	require.Equal(t, "us-east-1", lProd.Region)
}

func TestGlobalPath(t *testing.T) {
	p, err := GlobalPath()
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(p, filepath.Join(".config", "steer", "steer.toml")))
}
