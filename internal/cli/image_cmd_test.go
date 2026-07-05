package cli

import (
	"context"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/stretchr/testify/require"
)

func withFakeRegistry(t *testing.T, reg core.Registry) {
	t.Helper()
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: &coretest.FakeDeployer{CurrentTagValue: "v1"}, reg: reg}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
}

func sampleTags() []core.ImageTag {
	base := time.Now().Add(-2 * time.Hour)
	return []core.ImageTag{
		{Tag: "v2", Digest: "sha256:bbbb222222222", SizeBytes: 100 * 1024 * 1024, PushedAt: base.Add(time.Hour)},
		{Tag: "v1", Digest: "sha256:aaaa111111111", SizeBytes: 90 * 1024 * 1024, PushedAt: base},
	}
}

func TestImageLsListsRepos(t *testing.T) {
	withFakeRegistry(t, &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "nao-api"}, {Name: "nao-worker"}},
		Tags:  map[string][]core.ImageTag{"nao-api": sampleTags()},
	})
	out, err := runRoot(t, "image", "ls")
	require.NoError(t, err)
	require.Contains(t, out, "REPO")
	require.Contains(t, out, "nao-api")
	require.Contains(t, out, "nao-worker")
	require.Contains(t, out, "v2") // último tag del repo
}

func TestImageTagsListsTagsWithDeployedMarker(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{"api": sampleTags()}}
	withFakeRegistry(t, reg)
	out, err := runRoot(t, "image", "tags", "-r", "api")
	require.NoError(t, err)
	require.Contains(t, out, "TAG")
	require.Contains(t, out, "v2")
	require.Contains(t, out, "bbbb22222222") // digest corto
	require.Contains(t, out, "● now")        // v1 == CurrentTagValue del fake deployer
	require.Equal(t, []string{"api"}, reg.ListTagsCalls)
}

func TestImageTagsRequiresRepo(t *testing.T) {
	withFakeRegistry(t, &coretest.FakeRegistry{})
	_, err := runRoot(t, "image", "tags")
	require.ErrorContains(t, err, "--repo")
}

func TestImageWithoutConfigShowsHint(t *testing.T) {
	withFakeRegistry(t, nil) // fakeProvider.reg nil → ErrNoImagesConfig
	_, err := runRoot(t, "image", "ls")
	require.ErrorContains(t, err, "repo_template")
}
