package service

import (
	"context"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// reverseUserProbe records the account maps the panel pushes to the core: the
// last place a stored reverse tag can be dropped before it reaches a listener.
type reverseUserProbe struct {
	fakeNodeRuntime
	mu    sync.Mutex
	users []map[string]any
}

func (p *reverseUserProbe) AddUser(_ context.Context, _ *model.Inbound, user map[string]any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.users = append(p.users, user)
	return nil
}

func (p *reverseUserProbe) recorded() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]map[string]any(nil), p.users...)
}

const reverseProbeID = "5f2eb9d6-3a2f-4a55-9812-6ea1e2f7a333"

func reverseProbeClient(email string, enable bool) model.Client {
	return model.Client{Email: email, ID: reverseProbeID, Enable: enable, Reverse: &model.ClientReverse{Tag: "portal"}}
}

// assertReverseReAdd fails unless the core was handed the client's own tag: the
// handler is gone the moment RemoveUser runs, and only the tag rebuilds it.
func assertReverseReAdd(t *testing.T, probe *reverseUserProbe, email string) {
	t.Helper()
	users := probe.recorded()
	if len(users) == 0 {
		t.Fatalf("%s was never re-added to the core, so its reverse tag was never checked", email)
	}
	found := false
	for _, user := range users {
		if got, _ := user["email"].(string); got != email {
			continue
		}
		found = true
		tag, _ := user["reverse"].(*model.ClientReverse)
		if tag == nil || tag.Tag != "portal" {
			t.Fatalf("the re-add of %s carries reverse %#v, want its stored tag portal", email, user["reverse"])
		}
	}
	if !found {
		t.Fatalf("no re-add of %s reached the core: %v", email, users)
	}
}

// depleteClientTraffic leaves the client enabled in settings but out of quota,
// the state a traffic reset re-adds it from.
func depleteClientTraffic(t *testing.T, inboundId int, email string) {
	t.Helper()
	db := database.GetDB()
	res := db.Model(&xray.ClientTraffic{}).Where("email = ?", email).
		Updates(map[string]any{"enable": false, "up": 1, "down": 1})
	if res.Error != nil {
		t.Fatalf("deplete traffic: %v", res.Error)
	}
	if res.RowsAffected == 0 {
		if err := db.Create(&xray.ClientTraffic{InboundId: inboundId, Email: email, Enable: false, Up: 1, Down: 1}).Error; err != nil {
			t.Fatalf("create depleted traffic: %v", err)
		}
	}
}
