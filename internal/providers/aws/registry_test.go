package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/stretchr/testify/require"
)

// fakeECR devuelve páginas fijadas de repos e imágenes.
type fakeECR struct {
	repos           []ecrtypes.Repository
	images          []ecrtypes.ImageDetail
	imagesErr       error
	lastImagesInput *ecr.DescribeImagesInput
}

func (f *fakeECR) DescribeRepositories(_ context.Context, _ *ecr.DescribeRepositoriesInput, _ ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	return &ecr.DescribeRepositoriesOutput{Repositories: f.repos}, nil
}

func (f *fakeECR) DescribeImages(_ context.Context, in *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	f.lastImagesInput = in
	if f.imagesErr != nil {
		return nil, f.imagesErr
	}
	return &ecr.DescribeImagesOutput{ImageDetails: f.images}, nil
}

func TestListRepositoriesFiltersPrefixAndSorts(t *testing.T) {
	api := &fakeECR{repos: []ecrtypes.Repository{
		{RepositoryName: awssdk.String("nao-v2-worker")},
		{RepositoryName: awssdk.String("otro-equipo-api")},
		{RepositoryName: awssdk.String("nao-v2-api")},
	}}
	r := newRegistry(api, "nao-v2-")
	repos, err := r.ListRepositories(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 2)
	require.Equal(t, "nao-v2-api", repos[0].Name) // alfanumérico
	require.Equal(t, "nao-v2-worker", repos[1].Name)
}

func TestListTagsOnlyDeployableImages(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	img := func(tags []string, artifact string, pushed time.Time) ecrtypes.ImageDetail {
		d := ecrtypes.ImageDetail{
			ImageTags:        tags,
			ImageDigest:      awssdk.String("sha256:aaaa"),
			ImageSizeInBytes: awssdk.Int64(100 * 1024 * 1024),
			ImagePushedAt:    awssdk.Time(pushed),
		}
		if artifact != "" {
			d.ArtifactMediaType = awssdk.String(artifact)
		}
		return d
	}
	api := &fakeECR{images: []ecrtypes.ImageDetail{
		img([]string{"v1"}, "application/vnd.docker.container.image.v1+json", now.Add(-2*time.Hour)),
		img(nil, "", now),                                        // sin tag: fuera
		img([]string{"sha256-abc.sig"}, "", now),                 // firma cosign: fuera
		img([]string{"v2"}, "application/vnd.in-toto+json", now), // attestation: fuera
		img([]string{"v3", "latest"}, "application/vnd.oci.image.config.v1+json", now.Add(-time.Hour)),
	}}
	r := newRegistry(api, "")
	tags, err := r.ListTags(context.Background(), "nao-v2-api")
	require.NoError(t, err)
	// v3 y latest (misma imagen, 1h) antes que v1 (2h); nada más
	require.Len(t, tags, 3)
	require.Equal(t, "latest", tags[0].Tag) // empate por fecha → tag ascendente
	require.Equal(t, "v3", tags[1].Tag)
	require.Equal(t, "v1", tags[2].Tag)
	require.Equal(t, int64(100*1024*1024), tags[0].SizeBytes)
}

func TestHasTagFoundAndNotFound(t *testing.T) {
	api := &fakeECR{images: []ecrtypes.ImageDetail{{ImageTags: []string{"v1"}}}}
	r := newRegistry(api, "")
	ok, err := r.HasTag(context.Background(), "nao-v2-shared-api", "v1")
	require.NoError(t, err)
	require.True(t, ok)
	// la consulta es puntual: DescribeImages recibe el ImageIds con el tag
	require.Len(t, api.lastImagesInput.ImageIds, 1)
	require.Equal(t, "v1", awssdk.ToString(api.lastImagesInput.ImageIds[0].ImageTag))

	api.imagesErr = &ecrtypes.ImageNotFoundException{}
	ok, err = r.HasTag(context.Background(), "nao-v2-shared-api", "nope")
	require.NoError(t, err) // not found NO es error: es la respuesta
	require.False(t, ok)
}

func TestHasTagPropagatesRealErrors(t *testing.T) {
	api := &fakeECR{imagesErr: errors.New("throttled")}
	r := newRegistry(api, "")
	_, err := r.HasTag(context.Background(), "repo", "v1")
	require.ErrorContains(t, err, "throttled")
}
