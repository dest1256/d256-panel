package service

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// capPoolAtOneConn models a pool whose other connections are all parked on the
// transaction's row locks: any query outside tx then waits forever.
func capPoolAtOneConn(t *testing.T) {
	t.Helper()
	sqlDB, err := database.GetDB().DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
}

func runWithoutSecondConn(t *testing.T, name string, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		// The DB cleanup's Close fails the parked pool request, which unwinds fn.
		t.Fatalf("%s blocked waiting for a second pool connection while its transaction is open", name)
		return nil
	}
}

func TestUpdateClientTrafficsStaysOnOneConnection(t *testing.T) {
	setupBulkDB(t)
	svc := &InboundService{}
	clients := []model.Client{{Email: "gone@x", ID: "aaaaaaaa-0000-0000-0000-000000000101", Enable: true}}
	oldIb := mkInbound(t, 22101, model.VLESS, clientsSettings(t, clients))
	if err := database.GetDB().Create(&xray.ClientTraffic{InboundId: oldIb.Id, Email: "gone@x", Enable: true}).Error; err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
	newIb := *oldIb
	newIb.Settings = `{"clients": []}`

	capPoolAtOneConn(t)
	err := runWithoutSecondConn(t, "inbound edit dropping a client", func() error {
		return database.GetDB().Transaction(func(tx *gorm.DB) error {
			return svc.updateClientTraffics(tx, oldIb, &newIb)
		})
	})
	if err != nil {
		t.Fatalf("updateClientTraffics: %v", err)
	}
	var n int64
	database.GetDB().Model(xray.ClientTraffic{}).Where("email = ?", "gone@x").Count(&n)
	if n != 0 {
		t.Fatalf("traffic rows for dropped client = %d, want 0", n)
	}
}

func TestUpdateInboundClientRenameOntoExistingStatStaysOnOneConnection(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	inboundSvc := &InboundService{}
	source := []model.Client{{Email: "old@x", ID: "aaaaaaaa-0000-0000-0000-000000000102", SubID: "sub-102", Enable: true}}
	ib := mkInbound(t, 22102, model.VLESS, clientsSettings(t, source))
	if err := svc.SyncInbound(nil, ib.Id, source); err != nil {
		t.Fatalf("seed linkage: %v", err)
	}
	if err := database.GetDB().Create(&xray.ClientTraffic{InboundId: ib.Id, Email: "new@x", Enable: true}).Error; err != nil {
		t.Fatalf("seed target traffic: %v", err)
	}

	renamed := source
	renamed[0].Email = "new@x"
	capPoolAtOneConn(t)
	err := runWithoutSecondConn(t, "client rename onto an existing stats row", func() error {
		_, err := svc.UpdateInboundClient(inboundSvc, &model.Inbound{Id: ib.Id, Settings: clientsSettings(t, renamed)}, "old@x")
		return err
	})
	if err != nil {
		t.Fatalf("UpdateInboundClient: %v", err)
	}
	var n int64
	database.GetDB().Model(xray.ClientTraffic{}).Where("email = ?", "old@x").Count(&n)
	if n != 0 {
		t.Fatalf("traffic rows for renamed-away email = %d, want 0", n)
	}
}
