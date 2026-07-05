package aws

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

// fakeECR devuelve páginas fijadas de repos e imágenes.
type fakeECR struct {
	repos           []ecrtypes.Repository
	images          []ecrtypes.ImageDetail
	imagesErr       error
	lastImagesInput *ecr.DescribeImagesInput
	pageSize        int // >0: pagina las respuestas con NextToken (índice como token)
}

// paginate corta items en páginas de tamaño pageSize usando el índice como token.
func paginate[T any](items []T, token *string, pageSize int) (page []T, next *string) {
	start := 0
	if token != nil {
		start, _ = strconv.Atoi(*token)
	}
	end := min(start+pageSize, len(items))
	page = items[start:end]
	if end < len(items) {
		next = awssdk.String(strconv.Itoa(end))
	}
	return page, next
}

func (f *fakeECR) DescribeRepositories(_ context.Context, in *ecr.DescribeRepositoriesInput, _ ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	if f.pageSize > 0 {
		page, next := paginate(f.repos, in.NextToken, f.pageSize)
		return &ecr.DescribeRepositoriesOutput{Repositories: page, NextToken: next}, nil
	}
	return &ecr.DescribeRepositoriesOutput{Repositories: f.repos}, nil
}

func (f *fakeECR) DescribeImages(_ context.Context, in *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	f.lastImagesInput = in
	if f.imagesErr != nil {
		return nil, f.imagesErr
	}
	if f.pageSize > 0 {
		page, next := paginate(f.images, in.NextToken, f.pageSize)
		return &ecr.DescribeImagesOutput{ImageDetails: page, NextToken: next}, nil
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

func TestHasTagRepoNotFoundIsSentinel(t *testing.T) {
	api := &fakeECR{imagesErr: &ecrtypes.RepositoryNotFoundException{}}
	r := newRegistry(api, "")
	ok, err := r.HasTag(context.Background(), "nope-repo", "v1")
	require.False(t, ok)
	require.ErrorIs(t, err, core.ErrRepoNotFound)
}

func TestHasTagPropagatesRealErrors(t *testing.T) {
	api := &fakeECR{imagesErr: errors.New("throttled")}
	r := newRegistry(api, "")
	_, err := r.HasTag(context.Background(), "repo", "v1")
	require.ErrorContains(t, err, "throttled")
}

func TestListRepositoriesPaginates(t *testing.T) {
	api := &fakeECR{pageSize: 2, repos: []ecrtypes.Repository{
		{RepositoryName: awssdk.String("a")},
		{RepositoryName: awssdk.String("b")},
		{RepositoryName: awssdk.String("c")},
	}}
	repos, err := newRegistry(api, "").ListRepositories(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 3, "debe recorrer todas las páginas")
}

func TestListTagsPaginates(t *testing.T) {
	now := time.Now()
	img := func(tag string, d time.Duration) ecrtypes.ImageDetail {
		return ecrtypes.ImageDetail{ImageTags: []string{tag},
			ImageDigest: awssdk.String("sha256:x"), ImageSizeInBytes: awssdk.Int64(1),
			ImagePushedAt: awssdk.Time(now.Add(-d))}
	}
	api := &fakeECR{pageSize: 2, images: []ecrtypes.ImageDetail{
		img("v1", 3*time.Hour), img("v2", 2*time.Hour), img("v3", time.Hour),
	}}
	tags, err := newRegistry(api, "").ListTags(context.Background(), "repo")
	require.NoError(t, err)
	require.Len(t, tags, 3)
	require.Equal(t, "v3", tags[0].Tag) // el orden global sobrevive a la paginación
}
