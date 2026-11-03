package service

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	resetLostOn  = `{"clients":[{"email":"reset-lost","totalGB":100,"enable":true}]}`
	resetLostOff = `{"clients":[{"email":"reset-lost","totalGB":100,"enable":false}]}`
)

// resetRecordingRuntime is a node that accepts per-client resets unless failing.
type resetRecordingRuntime struct {
	fakeNodeRuntime
	mu   sync.Mutex
	fail bool
	got  []string
}

func (r *resetRecordingRuntime) ResetClientTraffic(_ context.Context, _ *model.Inbound, email string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.fail {
		return errors.New("node unreachable")
	}
	r.got = append(r.got, email)
	return nil
}

func (r *resetRecordingRuntime) delivered() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.got)
}

func pendingResetEmails(t *testing.T, nodeID int) []string {
	t.Helper()
	var emails []string
	if err := database.GetDB().Model(&model.NodePendingReset{}).Where("node_id = ?", nodeID).
		Order("email").Pluck("email", &emails).Error; err != nil {
		t.Fatalf("read pending resets: %v", err)
	}
	return emails
}

// bulkResetRuntime also takes a batch in one call.
type bulkResetRuntime struct {
	resetRecordingRuntime
	batches [][]string
}

func (b *bulkResetRuntime) ResetClientTraffics(_ context.Context, emails []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.batches = append(b.batches, slices.Clone(emails))
	return nil
}

// slowResetRuntime holds each reset until a second one arrives or a short
// timeout passes, so two unserialized deliveries both reach the node.
type slowResetRuntime struct {
	resetRecordingRuntime
	calls atomic.Int32
	both  chan struct{}
}

func (r *slowResetRuntime) ResetClientTraffic(ctx context.Context, ib *model.Inbound, email string) error {
	if r.calls.Add(1) == 2 {
		close(r.both)
	}
	select {
	case <-r.both:
	case <-time.After(300 * time.Millisecond):
	}
	return r.resetRecordingRuntime.ResetClientTraffic(ctx, ib, email)
}
