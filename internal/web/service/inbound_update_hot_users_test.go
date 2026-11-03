package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// inboundOpsProbe records every handler and user operation the panel sends the
// core, in order, and fails a remove or the first add the way xray does.
type inboundOpsProbe struct {
	fakeNodeRuntime
	mu        sync.Mutex
	ops       []string
	removeErr error
	addErr    error
}

func (p *inboundOpsProbe) record(op string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ops = append(p.ops, op)
}

func (p *inboundOpsProbe) AddInbound(context.Context, *model.Inbound) error {
	p.record("add-inbound")
	return nil
}

func (p *inboundOpsProbe) DelInbound(context.Context, *model.Inbound) error {
	p.record("del-inbound")
	return nil
}

func (p *inboundOpsProbe) UpdateInbound(context.Context, *model.Inbound, *model.Inbound) error {
	p.record("update-inbound")
	return nil
}

func (p *inboundOpsProbe) AddUser(_ context.Context, _ *model.Inbound, user map[string]any) error {
	email, _ := user["email"].(string)
	p.record("add-user " + email)
	p.mu.Lock()
	defer p.mu.Unlock()
	err := p.addErr
	p.addErr = nil
	return err
}

func (p *inboundOpsProbe) RemoveUser(_ context.Context, _ *model.Inbound, email string) error {
	p.record("remove-user " + email)
	return p.removeErr
}

func (p *inboundOpsProbe) recorded() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.ops...)
}

func hotUsersClient(email, id string, enable bool) model.Client {
	return model.Client{Email: email, ID: id, Enable: enable, CreatedAt: 1, UpdatedAt: 1}
}

func ssHotSettings(t *testing.T, method, serverKey string, clients []model.Client) string {
	t.Helper()
	var settings map[string]any
	if err := json.Unmarshal([]byte(clientsSettings(t, clients)), &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	settings["method"] = method
	settings["network"] = "tcp,udp"
	if serverKey != "" {
		settings["password"] = serverKey
	} else {
		for _, c := range settings["clients"].([]any) {
			c.(map[string]any)["method"] = method
		}
	}
	out, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal settings: %v", err)
	}
	return string(out)
}
