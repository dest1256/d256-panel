package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func mustUnmarshal(t *testing.T, raw string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		t.Fatalf("unmarshal %q: %v", raw, err)
	}
}

func settingsClientEmails(t *testing.T, inboundId int) []string {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundId).Error; err != nil {
		t.Fatalf("load inbound %d: %v", inboundId, err)
	}
	clients, err := (&InboundService{}).GetClients(&ib)
	if err != nil {
		t.Fatalf("GetClients: %v", err)
	}
	emails := make([]string, 0, len(clients))
	for _, c := range clients {
		emails = append(emails, c.Email)
	}
	return emails
}

func TestDedupeSettingsClients(t *testing.T) {
	dup := `{"clients": [` +
		`{"id": "u1", "email": "a@x", "subId": "s1"},` +
		`{"id": "u2", "email": "b@x", "subId": "s2"},` +
		`{"id": "u1", "email": "a@x", "subId": "s1"},` +
		`{"id": "u1", "email": "A@X", "subId": "s1"}]}`
	out, changed := dedupeSettingsClients(dup)
	if !changed {
		t.Fatal("expected duplicates to be removed")
	}
	var parsed struct {
		Clients []model.Client `json:"clients"`
	}
	mustUnmarshal(t, out, &parsed)
	if len(parsed.Clients) != 2 || parsed.Clients[0].Email != "a@x" || parsed.Clients[1].Email != "b@x" {
		t.Fatalf("expected first occurrences [a@x b@x], got %+v", parsed.Clients)
	}

	clean := `{"clients": [{"id": "u1", "email": "a@x"}, {"id": "u2", "email": "b@x"}]}`
	if _, changed := dedupeSettingsClients(clean); changed {
		t.Fatal("clean settings must not be rewritten")
	}
	if _, changed := dedupeSettingsClients(""); changed {
		t.Fatal("empty settings must not be rewritten")
	}
	if _, changed := dedupeSettingsClients("{not json"); changed {
		t.Fatal("invalid settings must not be rewritten")
	}
}
