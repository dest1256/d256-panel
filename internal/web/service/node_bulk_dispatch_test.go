package service

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// fakeNodeRuntime is a runtime.Runtime stub that counts the per-client dispatch
// calls so a test can assert a bulk op does NOT stream one RPC per client.
type fakeNodeRuntime struct {
	addInbound    atomic.Int32
	delInbound    atomic.Int32
	addClient     atomic.Int32
	deleteClient  atomic.Int32
	deleteUser    atomic.Int32
	updateInbound atomic.Int32
	updateSubSort atomic.Int32
	updateUser    atomic.Int32
}

func (f *fakeNodeRuntime) Name() string { return "fake-node" }

func (f *fakeNodeRuntime) AddInbound(context.Context, *model.Inbound) error {
	f.addInbound.Add(1)
	return nil
}

func (f *fakeNodeRuntime) DelInbound(context.Context, *model.Inbound) error {
	f.delInbound.Add(1)
	return nil
}

func (f *fakeNodeRuntime) UpdateInbound(context.Context, *model.Inbound, *model.Inbound) error {
	f.updateInbound.Add(1)
	return nil
}

func (f *fakeNodeRuntime) SetInboundSubSortIndex(context.Context, *model.Inbound, int) error {
	f.updateSubSort.Add(1)
	return nil
}

func (f *fakeNodeRuntime) AddUser(context.Context, *model.Inbound, map[string]any) error { return nil }

func (f *fakeNodeRuntime) RemoveUser(context.Context, *model.Inbound, string) error { return nil }

func (f *fakeNodeRuntime) UpdateUser(context.Context, *model.Inbound, string, model.Client) error {
	f.updateUser.Add(1)
	return nil
}

func (f *fakeNodeRuntime) DeleteUser(context.Context, *model.Inbound, string) error {
	f.deleteUser.Add(1)
	return nil
}

func (f *fakeNodeRuntime) DeleteClient(context.Context, string) error {
	f.deleteClient.Add(1)
	return nil
}

func (f *fakeNodeRuntime) AddClient(context.Context, *model.Inbound, model.Client) error {
	f.addClient.Add(1)
	return nil
}
func (f *fakeNodeRuntime) RestartXray(context.Context) error { return nil }
func (f *fakeNodeRuntime) ResetClientTraffic(context.Context, *model.Inbound, string) error {
	return nil
}
func (f *fakeNodeRuntime) ResetInboundTraffic(context.Context, *model.Inbound) error { return nil }
func (f *fakeNodeRuntime) ResetAllTraffics(context.Context) error                    { return nil }

// startSerializedWriter runs the single traffic-writer goroutine for the test, so
// concurrent service writes take the serialized path production uses.
func startSerializedWriter(t *testing.T) {
	t.Helper()
	resetTrafficWriterForTest(t)
	StartTrafficWriter()
}

// panicNodeRuntime panics on the per-client push, standing in for a bug in the
// apply path that would otherwise unwind straight out of a fanout goroutine.
type panicNodeRuntime struct{ fakeNodeRuntime }

func (p *panicNodeRuntime) AddClient(context.Context, *model.Inbound, model.Client) error {
	panic("boom from node runtime")
}

func nodeInbound(t *testing.T, nodeID, port int, clients []model.Client) *model.Inbound {
	t.Helper()
	if clients == nil {
		clients = []model.Client{}
	}
	ib := &model.Inbound{
		UserId: 1, NodeID: &nodeID, Tag: fmt.Sprintf("in-%d", port), Enable: true,
		Port: port, Protocol: model.VLESS, Settings: clientsSettings(t, clients),
	}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create node inbound: %v", err)
	}
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("seed SyncInbound: %v", err)
	}
	return ib
}

func makeNodeClients(n int) []model.Client {
	out := make([]model.Client, n)
	for i := range n {
		out[i] = model.Client{ID: uuid.NewString(), Email: fmt.Sprintf("nu-%05d@x", i), Enable: true}
	}
	return out
}
