package coretest

import (
	"context"
	"fmt"

	"github.com/juanMaAV92/steer/internal/core"
)

// FakeLogSource es un LogSource en memoria para tests: TailLogs devuelve
// Pages[0] y cada FollowLogs consume la página siguiente.
type FakeLogSource struct {
	Pages     []core.LogPage
	TailErr   error
	FollowErr error

	TailCalls   []string // "service/limit", en orden
	FollowCalls []string // "service/cursor", en orden

	next int
}

func (f *FakeLogSource) TailLogs(_ context.Context, service string, limit int) (core.LogPage, error) {
	f.TailCalls = append(f.TailCalls, fmt.Sprintf("%s/%d", service, limit))
	if f.TailErr != nil {
		return core.LogPage{}, f.TailErr
	}
	if len(f.Pages) == 0 {
		return core.LogPage{}, nil
	}
	f.next = 1
	return f.Pages[0], nil
}

func (f *FakeLogSource) FollowLogs(_ context.Context, service, cursor string) (core.LogPage, error) {
	f.FollowCalls = append(f.FollowCalls, service+"/"+cursor)
	if f.FollowErr != nil {
		return core.LogPage{}, f.FollowErr
	}
	if f.next >= len(f.Pages) {
		return core.LogPage{Cursor: cursor}, nil
	}
	p := f.Pages[f.next]
	f.next++
	return p, nil
}

var _ core.LogSource = (*FakeLogSource)(nil)
