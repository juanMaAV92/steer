package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvReturnsEnvironment(t *testing.T) {
	cfg := &Config{Providers: Providers{AWS: AWS{
		Environments: map[string]Environment{"stg": {Profile: "staging"}},
	}}}

	env, err := cfg.Env("stg")
	require.NoError(t, err)
	require.Equal(t, "staging", env.Profile)
}

func TestEnvUnknown(t *testing.T) {
	cfg := &Config{}
	_, err := cfg.Env("ghost")
	require.ErrorContains(t, err, "ghost")
}

func TestNamingDefaults(t *testing.T) {
	var n Naming // plantillas vacías → defaults
	require.Equal(t, "stg-cluster", n.Cluster("stg"))
	require.Equal(t, "catalog", n.Service("catalog"))
}

func TestNamingTemplates(t *testing.T) {
	n := Naming{ClusterTemplate: "prod-{env}-ecs", ServiceTemplate: "svc-{name}"}
	require.Equal(t, "prod-stg-ecs", n.Cluster("stg"))
	require.Equal(t, "svc-catalog", n.Service("catalog"))
}

func TestCandidatePaths(t *testing.T) {
	got := candidatePaths("/work", "/home/u")
	require.Equal(t, []string{
		"/work/steer.toml",
		"/home/u/.config/steer/steer.toml",
	}, got)
}
