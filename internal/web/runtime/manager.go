// Package runtime is the seam between inbound/client services and whatever runs the inbounds.
//
// Nodes are state-driven: the node link (internal/nodelink) renders each node's desired state from the
// database and pushes it whenever it changes, so the services no longer apply changes imperatively.
// Every operation here is therefore a no-op; the interface stays so the call sites can be removed
// one at a time without a flag day.
package runtime

import (
	"context"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

type noop struct{}

func (noop) Name() string                                                           { return "node-link" }
func (noop) AddInbound(context.Context, *model.Inbound) error                       { return nil }
func (noop) DelInbound(context.Context, *model.Inbound) error                       { return nil }
func (noop) UpdateInbound(context.Context, *model.Inbound, *model.Inbound) error    { return nil }
func (noop) AddUser(context.Context, *model.Inbound, map[string]any) error          { return nil }
func (noop) RemoveUser(context.Context, *model.Inbound, string) error               { return nil }
func (noop) UpdateUser(context.Context, *model.Inbound, string, model.Client) error { return nil }
func (noop) DeleteUser(context.Context, *model.Inbound, string) error               { return nil }
func (noop) AddClient(context.Context, *model.Inbound, model.Client) error          { return nil }
func (noop) DeleteClient(context.Context, string) error                             { return nil }
func (noop) RestartXray(context.Context) error                                      { return nil }
func (noop) ResetClientTraffic(context.Context, *model.Inbound, string) error       { return nil }
func (noop) ResetInboundTraffic(context.Context, *model.Inbound) error              { return nil }
func (noop) ResetAllTraffics(context.Context) error                                 { return nil }

// Manager hands out the (no-op) runtime for an inbound's node.
type Manager struct{}

func NewManager() *Manager { return &Manager{} }

func (*Manager) RuntimeFor(*int) (Runtime, error) { return noop{}, nil }

func (*Manager) Local() Runtime { return noop{} }

// InvalidateNode is kept for callers that dropped a node; there is nothing cached any more.
func (*Manager) InvalidateNode(int) {}

var (
	managerMu sync.RWMutex
	manager   *Manager
)

func SetManager(m *Manager) {
	managerMu.Lock()
	defer managerMu.Unlock()
	manager = m
}

func GetManager() *Manager {
	managerMu.RLock()
	defer managerMu.RUnlock()
	return manager
}
