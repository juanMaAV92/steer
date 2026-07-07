package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeAWSFixtures(t *testing.T) string {
	home := t.TempDir()
	dir := filepath.Join(home, ".aws")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config"), []byte(`[default]
region = us-east-1

[profile dev]
sso_session = corp

[profile staging]
region = us-west-2
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials"), []byte(`[legacy]
aws_access_key_id = AKIA...

[dev]
aws_access_key_id = AKIA...
`), 0o600))
	return home
}

func TestDetectorProfilesParsesAndDedups(t *testing.T) {
	d := NewDetectorWithHome(writeAWSFixtures(t))
	profiles, err := d.Profiles()
	require.NoError(t, err)
	// default + dev + staging (config) + legacy (credentials); dev deduplicado; orden alfabético
	require.Equal(t, []string{"default", "dev", "legacy", "staging"}, profiles)
}

func TestDetectorProfilesNoAWSDir(t *testing.T) {
	d := NewDetectorWithHome(t.TempDir())
	profiles, err := d.Profiles()
	require.NoError(t, err) // sin ~/.aws no es error: lista vacía (el wizard enseña)
	require.Empty(t, profiles)
}

func TestDetectorClustersListsNames(t *testing.T) {
	api := &fakeECS{clusterArns: []string{
		"arn:aws:ecs:us-east-1:1:cluster/nao-v2-dev-cluster",
		"arn:aws:ecs:us-east-1:1:cluster/otro",
	}}
	d := NewDetectorWithHome(t.TempDir())
	d.newECS = func(context.Context, string, string) (ecsAPI, error) { return api, nil }
	clusters, err := d.Clusters(context.Background(), "dev", "")
	require.NoError(t, err)
	require.Equal(t, []string{"nao-v2-dev-cluster", "otro"}, clusters)
}
