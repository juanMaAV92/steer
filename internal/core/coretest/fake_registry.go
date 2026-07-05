package coretest

import (
	"context"

	"github.com/juanMaAV92/steer/internal/core"
)

// FakeRegistry es un Registry en memoria para tests.
type FakeRegistry struct {
	Repos    []core.Repository
	Tags     map[string][]core.ImageTag
	ReposErr error
	TagsErr  error

	ListTagsCalls []string // repos consultados, en orden
}

func (f *FakeRegistry) ListRepositories(_ context.Context) ([]core.Repository, error) {
	return f.Repos, f.ReposErr
}

func (f *FakeRegistry) ListTags(_ context.Context, repo string) ([]core.ImageTag, error) {
	f.ListTagsCalls = append(f.ListTagsCalls, repo)
	if f.TagsErr != nil {
		return nil, f.TagsErr
	}
	return f.Tags[repo], nil
}
