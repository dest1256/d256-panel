package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// fanoutGate holds every node call open until released, so a test sees how many
// nodes an operation reaches at once.
type fanoutGate struct {
	entered atomic.Int32
	release chan struct{}
	once    sync.Once
}

func newFanoutGate() *fanoutGate { return &fanoutGate{release: make(chan struct{})} }

func (g *fanoutGate) hold(ctx context.Context) error {
	g.entered.Add(1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.release:
		return nil
	}
}

func (g *fanoutGate) open() { g.once.Do(func() { close(g.release) }) }

func (g *fanoutGate) waitAll(t *testing.T, want int32, op string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for g.entered.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("%s reached %d of %d hanging nodes, want all of them at once", op, g.entered.Load(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

type gatedNodeRuntime struct {
	fakeNodeRuntime
	gate *fanoutGate
}

func (r *gatedNodeRuntime) ResetAllTraffics(ctx context.Context) error { return r.gate.hold(ctx) }

func (r *gatedNodeRuntime) DelInbound(ctx context.Context, _ *model.Inbound) error {
	return r.gate.hold(ctx)
}
