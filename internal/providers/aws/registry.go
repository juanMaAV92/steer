package aws

import (
	"context"
	"errors"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/juanMaAV92/steer/internal/core"
)

// maxTags limita cuántas imágenes devuelve ListTags (las más recientes).
const maxTags = 50

// ecrAPI es el subconjunto del cliente ECR que usa el registry.
// El *ecr.Client del SDK lo satisface; los tests inyectan un fake.
type ecrAPI interface {
	DescribeRepositories(ctx context.Context, in *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	DescribeImages(ctx context.Context, in *ecr.DescribeImagesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error)
}

// ECRRegistry implementa core.Registry sobre AWS ECR.
type ECRRegistry struct {
	api    ecrAPI
	prefix string // prefijo de repos del contexto (config.Context.RepoPrefix)
}

// NewRegistry crea un ECRRegistry desde una aws.Config.
func NewRegistry(cfg awssdk.Config, prefix string) *ECRRegistry {
	return newRegistry(ecr.NewFromConfig(cfg), prefix)
}

// newRegistry es el constructor inyectable usado por los tests.
func newRegistry(api ecrAPI, prefix string) *ECRRegistry {
	return &ECRRegistry{api: api, prefix: prefix}
}

// ListRepositories devuelve los repos que casan con el prefijo, alfanuméricos.
func (r *ECRRegistry) ListRepositories(ctx context.Context) ([]core.Repository, error) {
	var out []core.Repository
	var token *string
	for {
		resp, err := r.api.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, repo := range resp.Repositories {
			name := awssdk.ToString(repo.RepositoryName)
			if r.prefix == "" || strings.HasPrefix(name, r.prefix) {
				out = append(out, core.Repository{Name: name})
			}
		}
		if awssdk.ToString(resp.NextToken) == "" {
			break
		}
		token = resp.NextToken
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// deployableArtifact acepta solo media types de imagen de contenedor real
// (vacío = manifiestos docker clásicos sin artifact type).
func deployableArtifact(mediaType string) bool {
	switch mediaType {
	case "", "application/vnd.docker.container.image.v1+json", "application/vnd.oci.image.config.v1+json":
		return true
	}
	return false
}

// signatureTag detecta tags de firma/attestation por convención cosign.
func signatureTag(tag string) bool {
	return strings.HasSuffix(tag, ".sig") || strings.HasSuffix(tag, ".att")
}

// ListTags devuelve solo imágenes con tag desplegables, más recientes primero.
func (r *ECRRegistry) ListTags(ctx context.Context, repo string) ([]core.ImageTag, error) {
	var out []core.ImageTag
	var token *string
	for {
		resp, err := r.api.DescribeImages(ctx, &ecr.DescribeImagesInput{
			RepositoryName: awssdk.String(repo),
			NextToken:      token,
		})
		if err != nil {
			return nil, err
		}
		for _, img := range resp.ImageDetails {
			if len(img.ImageTags) == 0 || !deployableArtifact(awssdk.ToString(img.ArtifactMediaType)) {
				continue // manifiesto colgante o attestation/SBOM: no es imagen desplegable
			}
			for _, tag := range img.ImageTags {
				if signatureTag(tag) {
					continue
				}
				out = append(out, core.ImageTag{
					Tag:       tag,
					Digest:    awssdk.ToString(img.ImageDigest),
					SizeBytes: awssdk.ToInt64(img.ImageSizeInBytes),
					PushedAt:  awssdk.ToTime(img.ImagePushedAt),
				})
			}
		}
		if awssdk.ToString(resp.NextToken) == "" {
			break
		}
		token = resp.NextToken
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].PushedAt.Equal(out[j].PushedAt) {
			return out[i].PushedAt.After(out[j].PushedAt)
		}
		return out[i].Tag < out[j].Tag
	})
	if len(out) > maxTags {
		out = out[:maxTags]
	}
	return out, nil
}

// HasTag verifica el tag con una consulta puntual; ImageNotFoundException es la
// respuesta "no existe", no un error.
func (r *ECRRegistry) HasTag(ctx context.Context, repo, tag string) (bool, error) {
	_, err := r.api.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RepositoryName: awssdk.String(repo),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageTag: awssdk.String(tag)}},
	})
	var repoNotFound *ecrtypes.RepositoryNotFoundException
	if errors.As(err, &repoNotFound) {
		return false, core.ErrRepoNotFound
	}
	var notFound *ecrtypes.ImageNotFoundException
	if errors.As(err, &notFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
