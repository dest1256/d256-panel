package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func kernelAWGSettings(clients []model.Client) string {
	b, _ := json.Marshal(map[string]any{
		"server":  map[string]any{"subnetIp": "10.8.1.0", "subnetCidr": 24, "kernelMode": true},
		"clients": clients,
	})
	return string(b)
}

func mustAWGClients(t *testing.T, ib *model.Inbound) []model.Client {
	t.Helper()
	clients, err := (&InboundService{}).GetClients(ib)
	if err != nil {
		t.Fatal(err)
	}
	return clients
}
