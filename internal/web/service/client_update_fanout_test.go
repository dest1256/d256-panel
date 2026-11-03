package service

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// applyBarrierRuntime holds every armed node push until fanout of them are
// inside it at once; a sequential caller only ever reaches one and times out.
// It stays pass-through until arm() so a test can seed its clients first.
type applyBarrierRuntime struct {
	fakeNodeRuntime
	fanout   int32
	armed    atomic.Bool
	inFlight atomic.Int32
	maxPar   atomic.Int32
	release  chan struct{}
	freed    atomic.Bool
	expired  atomic.Bool
}

func newApplyBarrier(fanout int32) *applyBarrierRuntime {
	return &applyBarrierRuntime{fanout: fanout, release: make(chan struct{})}
}

func (b *applyBarrierRuntime) arm() { b.armed.Store(true) }

func (b *applyBarrierRuntime) free() {
	if b.freed.CompareAndSwap(false, true) {
		close(b.release)
	}
}

func (b *applyBarrierRuntime) wait() {
	if !b.armed.Load() {
		return
	}
	n := b.inFlight.Add(1)
	for {
		peak := b.maxPar.Load()
		if n <= peak || b.maxPar.CompareAndSwap(peak, n) {
			break
		}
	}
	if n == b.fanout {
		b.free()
	}
	select {
	case <-b.release:
	case <-time.After(5 * time.Second):
		// Release everyone on the first timeout so a sequential regression
		// fails once instead of stalling for fanout x the wait.
		b.expired.Store(true)
		b.free()
	}
	b.inFlight.Add(-1)
}

func (b *applyBarrierRuntime) UpdateUser(ctx context.Context, ib *model.Inbound, oldEmail string, c model.Client) error {
	b.wait()
	return b.fakeNodeRuntime.UpdateUser(ctx, ib, oldEmail, c)
}

func (b *applyBarrierRuntime) AddClient(ctx context.Context, ib *model.Inbound, c model.Client) error {
	b.wait()
	return b.fakeNodeRuntime.AddClient(ctx, ib, c)
}

func (b *applyBarrierRuntime) ResetClientTraffic(ctx context.Context, ib *model.Inbound, email string) error {
	b.wait()
	return b.fakeNodeRuntime.ResetClientTraffic(ctx, ib, email)
}

func (b *applyBarrierRuntime) DeleteClient(ctx context.Context, email string) error {
	b.wait()
	return b.fakeNodeRuntime.DeleteClient(ctx, email)
}

func (b *applyBarrierRuntime) DeleteUser(ctx context.Context, ib *model.Inbound, email string) error {
	b.wait()
	return b.fakeNodeRuntime.DeleteUser(ctx, ib, email)
}
