package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestEnsureInboundTagAllowed(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	svc := NodeService{}

	selected := &model.Node{
		Name: "ensure-sel", Address: "127.0.0.1", Port: 2096, ApiToken: "tok",
		InboundSyncMode: "selected", InboundTags: []string{"a"},
	}
	if err := db.Create(selected).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	if err := svc.EnsureInboundTagAllowed(selected.Id, "b"); err != nil {
		t.Fatalf("EnsureInboundTagAllowed add: %v", err)
	}
	var got model.Node
	if err := db.First(&got, selected.Id).Error; err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if len(got.InboundTags) != 2 || got.InboundTags[0] != "a" || got.InboundTags[1] != "b" {
		t.Fatalf("InboundTags = %#v, want [a b]", got.InboundTags)
	}

	if err := svc.EnsureInboundTagAllowed(selected.Id, "a"); err != nil {
		t.Fatalf("EnsureInboundTagAllowed existing: %v", err)
	}
	if err := db.First(&got, selected.Id).Error; err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if len(got.InboundTags) != 2 {
		t.Fatalf("existing tag must not duplicate, got %#v", got.InboundTags)
	}

	all := &model.Node{
		Name: "ensure-all", Address: "127.0.0.1", Port: 2097, ApiToken: "tok",
		InboundSyncMode: "all",
	}
	if err := db.Create(all).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := svc.EnsureInboundTagAllowed(all.Id, "x"); err != nil {
		t.Fatalf("EnsureInboundTagAllowed all-mode: %v", err)
	}
	var gotAll model.Node
	if err := db.First(&gotAll, all.Id).Error; err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if len(gotAll.InboundTags) != 0 {
		t.Fatalf("all-mode node must stay without tags, got %#v", gotAll.InboundTags)
	}
}
