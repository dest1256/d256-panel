package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

type hangingUpdateRuntime struct {
	fakeNodeRuntime
	entered chan struct{}
	release chan struct{}
}

func (h *hangingUpdateRuntime) UpdateInbound(ctx context.Context, _, _ *model.Inbound) error {
	h.updateInbound.Add(1)
	select {
	case h.entered <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.release:
		return nil
	}
}

func seedDepletedNodeClient(t *testing.T, nodeID, port int) {
	t.Helper()
	client := model.Client{Email: fmt.Sprintf("spent-%d", port), Enable: true}
	ib := nodeInbound(t, nodeID, port, []model.Client{client})
	if err := database.GetDB().Create(&xray.ClientTraffic{
		InboundId: ib.Id, Email: client.Email, Enable: true, Up: 100, Total: 100,
	}).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
}

type hangingRestartRuntime struct {
	fakeNodeRuntime
	entered chan struct{}
	release chan struct{}
}

func (h *hangingRestartRuntime) RestartXray(ctx context.Context) error {
	select {
	case h.entered <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-h.release:
		return nil
	}
}
