package service

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestCreateAcrossManyInboundsUsesOneEmailSnapshot(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}

	const uuid = "bbbbbbbb-1111-2222-3333-555555555555"
	ids := make([]int, 0, 6)
	for i := range 6 {
		ib := mkInbound(t, 23001+i, model.VLESS, `{"clients":[]}`)
		ids = append(ids, ib.Id)
	}

	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "fan@x", ID: uuid, SubID: "sub-fan", Enable: true},
		InboundIds: ids,
	}); err != nil {
		t.Fatalf("Create across %d inbounds: %v", len(ids), err)
	}

	if n := countClientRecords(t); n != 1 {
		t.Fatalf("client records = %d, want 1", n)
	}
	rec := lookupClientRecord(t, "fan@x")
	if rec.UUID != uuid || rec.SubID != "sub-fan" {
		t.Fatalf("record = {uuid:%q sub:%q}, want {%q sub-fan}", rec.UUID, rec.SubID, uuid)
	}
	for _, id := range ids {
		if !settingsHoldUUID(t, inboundSvc, id, uuid) {
			t.Fatalf("inbound %d settings missing the client", id)
		}
	}

	linked, err := svc.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		t.Fatalf("GetInboundIdsForRecord: %v", err)
	}
	if len(linked) != len(ids) {
		t.Fatalf("linked inbounds = %d, want %d", len(linked), len(ids))
	}
}

func TestAttachAcrossManyInboundsUsesOneEmailSnapshot(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}

	first := mkInbound(t, 23101, model.VLESS, `{"clients":[]}`)
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "att@x", ID: "cccccccc-1111-2222-3333-666666666666", SubID: "sub-att", Enable: true},
		InboundIds: []int{first.Id},
	}); err != nil {
		t.Fatalf("seed Create: %v", err)
	}
	rec := lookupClientRecord(t, "att@x")

	ids := []int{first.Id}
	for i := range 4 {
		ib := mkInbound(t, 23102+i, model.VLESS, `{"clients":[]}`)
		ids = append(ids, ib.Id)
	}

	if _, err := svc.Attach(inboundSvc, rec.Id, ids); err != nil {
		t.Fatalf("Attach across %d inbounds: %v", len(ids), err)
	}

	if n := countClientRecords(t); n != 1 {
		t.Fatalf("client records after attach = %d, want 1", n)
	}
	linked, err := svc.GetInboundIdsForRecord(rec.Id)
	if err != nil {
		t.Fatalf("GetInboundIdsForRecord: %v", err)
	}
	if len(linked) != len(ids) {
		t.Fatalf("linked inbounds = %d, want %d", len(linked), len(ids))
	}
}

// barrierNodeRuntime holds every AddClient until fanout of them are inside it at
// once, recording the peak overlap; a sequential caller only ever reaches one.
type barrierNodeRuntime struct {
	fakeNodeRuntime
	fanout   int32
	inFlight atomic.Int32
	maxPar   atomic.Int32
	release  chan struct{}
	freed    atomic.Bool
	expired  atomic.Bool
}

func (b *barrierNodeRuntime) free() {
	if b.freed.CompareAndSwap(false, true) {
		close(b.release)
	}
}

func (b *barrierNodeRuntime) AddClient(ctx context.Context, ib *model.Inbound, c model.Client) error {
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
	return b.fakeNodeRuntime.AddClient(ctx, ib, c)
}

// TestCreateLeavesHwidLimitAloneWhenCreateFails pins that a create the panel
// reported as failed never rewrites a device cap, so it can never retrim one.
func TestCreateLeavesHwidLimitAloneWhenCreateFails(t *testing.T) {
	setupBulkDB(t)
	startSerializedWriter(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}

	const vipUUID = "44444444-5555-6666-7777-888888888888"
	seed := mkInbound(t, 41401, model.VLESS, `{"clients":[]}`)
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "vip@x", ID: vipUUID, SubID: "sub-vip", Enable: true},
		InboundIds: []int{seed.Id},
		LimitHwid:  3,
	}); err != nil {
		t.Fatalf("seed Create: %v", err)
	}

	broken := mkInbound(t, 41402, model.VLESS, `{"clients":`)
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "vip@x", ID: vipUUID, SubID: "sub-vip", Enable: true},
		InboundIds: []int{broken.Id},
		LimitHwid:  1,
	}); err == nil {
		t.Fatal("re-adding to an unparsable inbound returned no error")
	}

	if rec := lookupClientRecord(t, "vip@x"); rec.LimitHwid != 3 {
		t.Fatalf("limit_hwid = %d, want the untouched 3: a failed create retrimmed a live client", rec.LimitHwid)
	}

	// Same failure with the seeded inbound alongside it: that one is a dedup
	// no-op returning no error, which must not read as "an inbound took it".
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "vip@x", ID: vipUUID, SubID: "sub-vip", Enable: true},
		InboundIds: []int{seed.Id, broken.Id},
		LimitHwid:  1,
	}); err == nil {
		t.Fatal("re-adding over a no-op and an unparsable inbound returned no error")
	}
	if rec := lookupClientRecord(t, "vip@x"); rec.LimitHwid != 3 {
		t.Fatalf("limit_hwid = %d, want the untouched 3: a no-op inbound counted as applied", rec.LimitHwid)
	}

	// A brand new identity that only partly applies is left uncapped rather than
	// capped, the deliberate safe side: the operator saw the error and retries.
	healthy := mkInbound(t, 41403, model.VLESS, `{"clients":[]}`)
	if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
		Client:     model.Client{Email: "fresh@x", ID: "55555555-6666-7777-8888-999999999999", SubID: "sub-fresh", Enable: true},
		InboundIds: []int{healthy.Id, broken.Id},
		LimitHwid:  5,
	}); err == nil {
		t.Fatal("creating over an unparsable inbound returned no error")
	}
	if rec := lookupClientRecord(t, "fresh@x"); rec.LimitHwid != 0 {
		t.Fatalf("limit_hwid = %d, want 0 on a create that failed", rec.LimitHwid)
	}
}

func assertNamesFailedInbounds(t *testing.T, err error, broken []*model.Inbound, healthy *model.Inbound) {
	t.Helper()
	if err == nil {
		t.Fatalf("applying %d unparsable inbounds returned no error", len(broken))
	}
	for _, ib := range broken {
		if want := fmt.Sprintf("inbound %d:", ib.Id); !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name the failing %s", err, want)
		}
	}
	if blamed := fmt.Sprintf("inbound %d:", healthy.Id); strings.Contains(err.Error(), blamed) {
		t.Fatalf("error %q blames the healthy %s", err, blamed)
	}
}

// TestFanoutReportsEveryFailingInbound pins that no inbound aborts the others:
// each failure names its own inbound, and the healthy ones still get the client.
func TestFanoutReportsEveryFailingInbound(t *testing.T) {
	const halfBadUUID = "22222222-3333-4444-5555-666666666666"

	t.Run("create", func(t *testing.T) {
		setupBulkDB(t)
		startSerializedWriter(t)
		svc := &ClientService{}
		inboundSvc := &InboundService{}

		broken := []*model.Inbound{
			mkInbound(t, 41201, model.VLESS, `{"clients":`),
			mkInbound(t, 41202, model.VLESS, `{"clients":`),
		}
		healthy := mkInbound(t, 41203, model.VLESS, `{"clients":[]}`)

		_, err := svc.Create(inboundSvc, &ClientCreatePayload{
			Client:     model.Client{Email: "halfbad@x", ID: halfBadUUID, SubID: "sub-halfbad", Enable: true},
			InboundIds: []int{broken[0].Id, broken[1].Id, healthy.Id},
		})
		assertNamesFailedInbounds(t, err, broken, healthy)
		if !settingsHoldUUID(t, inboundSvc, healthy.Id, halfBadUUID) {
			t.Fatalf("healthy inbound %d did not get the client", healthy.Id)
		}
	})

	t.Run("attach", func(t *testing.T) {
		setupBulkDB(t)
		startSerializedWriter(t)
		svc := &ClientService{}
		inboundSvc := &InboundService{}

		seed := mkInbound(t, 41301, model.VLESS, `{"clients":[]}`)
		if _, err := svc.Create(inboundSvc, &ClientCreatePayload{
			Client:     model.Client{Email: "halfbad@x", ID: halfBadUUID, SubID: "sub-halfbad", Enable: true},
			InboundIds: []int{seed.Id},
		}); err != nil {
			t.Fatalf("seed Create: %v", err)
		}

		broken := []*model.Inbound{
			mkInbound(t, 41302, model.VLESS, `{"clients":`),
			mkInbound(t, 41303, model.VLESS, `{"clients":`),
		}
		healthy := mkInbound(t, 41304, model.VLESS, `{"clients":[]}`)

		rec := lookupClientRecord(t, "halfbad@x")
		_, err := svc.Attach(inboundSvc, rec.Id, []int{broken[0].Id, broken[1].Id, healthy.Id})
		assertNamesFailedInbounds(t, err, broken, healthy)
		if !settingsHoldUUID(t, inboundSvc, healthy.Id, halfBadUUID) {
			t.Fatalf("healthy inbound %d did not get the client", healthy.Id)
		}
	})
}
