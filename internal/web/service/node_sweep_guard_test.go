package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"gorm.io/gorm"
)

func countClientRows(t *testing.T, db *gorm.DB, email string) (records, traffics int64) {
	t.Helper()
	if err := db.Model(&model.ClientRecord{}).Where("email = ?", email).Count(&records).Error; err != nil {
		t.Fatalf("count clients %q: %v", email, err)
	}
	if err := db.Model(xray.ClientTraffic{}).Where("email = ?", email).Count(&traffics).Error; err != nil {
		t.Fatalf("count client_traffics %q: %v", email, err)
	}
	return records, traffics
}

func seedNodeRow(t *testing.T, db *gorm.DB, n *model.Node) {
	t.Helper()
	if err := db.Create(n).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
}

func TestSyncInboundStoresTrimmedEmail(t *testing.T) {
	db := initTrafficTestDB(t)
	svc := &ClientService{}

	ib := &model.Inbound{UserId: 1, Tag: "trim-in", Enable: true, Port: 41501, Protocol: model.VLESS}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	padded := "bob "
	clients := []model.Client{{Email: padded, Enable: true}}
	if err := svc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("first SyncInbound: %v", err)
	}

	var stored []string
	if err := db.Model(&model.ClientRecord{}).Pluck("email", &stored).Error; err != nil {
		t.Fatalf("read clients: %v", err)
	}
	if len(stored) != 1 || stored[0] != "bob" {
		t.Fatalf("stored email = %q, want the trimmed %q — the lookup key must match what is written", stored, "bob")
	}

	if err := svc.SyncInbound(nil, ib.Id, clients); err != nil {
		t.Fatalf("second SyncInbound must not hit a unique-constraint on the untrimmed row: %v", err)
	}
	var links int64
	if err := db.Model(&model.ClientInbound{}).Where("inbound_id = ?", ib.Id).Count(&links).Error; err != nil {
		t.Fatalf("count links: %v", err)
	}
	if links != 1 {
		t.Fatalf("links after re-sync = %d, want 1", links)
	}
}
