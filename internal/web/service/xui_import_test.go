package service

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const (
	importUUID1 = "6b0b0e3e-41b2-4f1c-9a16-0d5c1c1f3a11"
	importUUID2 = "0c1c4b34-7d68-4d6a-8d5e-3f7a1a1b9c22"
)

// legacyXUIDB writes a database shaped like a 3x-ui x-ui.db: inbounds with their clients inside the
// settings JSON, and a client_traffics table with usage.
func legacyXUIDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "x-ui.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Inbound{}, &xray.ClientTraffic{}); err != nil {
		t.Fatal(err)
	}
	vless := &model.Inbound{
		Remark: "main", Enable: true, Port: 24443, Protocol: model.VLESS, Tag: "inbound-24443",
		Settings: `{"clients":[` +
			`{"id":"` + importUUID1 + `","email":"alice@old","enable":true,"limitIp":2,"totalGB":1000,"expiryTime":0,"subId":"oldsub1"},` +
			`{"id":"` + importUUID2 + `","email":"bob@old","enable":true,"subId":"oldsub2"}],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Sniffing:       `{"enabled":false}`,
	}
	mtproto := &model.Inbound{
		Remark: "mt", Enable: true, Port: 24444, Protocol: "mtproto", Tag: "inbound-24444",
		Settings: `{"clients":[]}`, StreamSettings: `{}`, Sniffing: `{}`,
	}
	clash := &model.Inbound{
		Remark: "clash", Enable: true, Port: 24443, Protocol: model.VLESS, Tag: "inbound-24443b",
		Settings: `{"clients":[],"decryption":"none"}`, StreamSettings: `{"network":"tcp","security":"none"}`, Sniffing: `{}`,
	}
	for _, in := range []*model.Inbound{vless, mtproto, clash} {
		if err := db.Create(in).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Create(&xray.ClientTraffic{InboundId: vless.Id, Enable: true, Email: "alice@old", Up: 111, Down: 222, Total: 1000, ExpiryTime: 1893456000000})
	db.Create(&xray.ClientTraffic{InboundId: vless.Id, Enable: true, Email: "bob@old", Up: 5, Down: 6})
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	return path
}

func TestImportXUIDBAddsInboundsClientsAndTraffic(t *testing.T) {
	setupBulkDB(t)
	node := &model.Node{Name: "target", Address: "n.example", Enable: true, Status: "pending"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	// Something that already exists on the panel must survive the import.
	keep := &model.Inbound{
		Remark: "keep", Enable: true, Port: 30001, Protocol: model.VLESS, Tag: "keep-30001",
		Settings: `{"clients":[],"decryption":"none"}`, StreamSettings: `{"network":"tcp","security":"none"}`, Sniffing: `{}`,
	}
	if err := database.GetDB().Create(keep).Error; err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(legacyXUIDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var svc InboundService
	res, err := svc.ImportXUIDB(f, node.Id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inbounds != 1 || res.Clients != 2 || len(res.Skipped) != 2 {
		t.Fatalf("result = %+v", res)
	}
	reasons := res.Skipped[0].Reason + " | " + res.Skipped[1].Reason
	if res.Skipped[0].Remark != "mt" || res.Skipped[1].Remark != "clash" {
		t.Fatalf("skipped = %s", reasons)
	}

	var imported model.Inbound
	if err := database.GetDB().Where("remark = ?", "main").First(&imported).Error; err != nil {
		t.Fatal(err)
	}
	if imported.NodeID == nil || *imported.NodeID != node.Id || imported.Port != 24443 {
		t.Fatalf("inbound = %+v", imported)
	}
	var alice xray.ClientTraffic
	database.GetDB().Where("email = ?", "alice@old").First(&alice)
	if alice.Up != 111 || alice.Down != 222 || alice.Total != 1000 || alice.ExpiryTime != 1893456000000 || alice.InboundId != imported.Id {
		t.Fatalf("alice traffic = %+v", alice)
	}
	var rec model.ClientRecord
	if err := database.GetDB().Where("email = ?", "alice@old").First(&rec).Error; err != nil || rec.SubID != "oldsub1" {
		t.Fatalf("client record = %+v err=%v", rec, err)
	}
	var kept int64
	database.GetDB().Model(&model.Inbound{}).Where("tag = ?", "keep-30001").Count(&kept)
	if kept != 1 {
		t.Fatal("existing inbound was lost")
	}

	// Importing the same file again must not create duplicates: the port is already taken on the node.
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	again, err := svc.ImportXUIDB(f, node.Id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if again.Inbounds != 0 || len(again.Skipped) != 3 {
		t.Fatalf("second import = %+v", again)
	}
}

func TestImportXUIDBRejectsBadInput(t *testing.T) {
	setupBulkDB(t)
	node := &model.Node{Name: "target", Address: "n.example", Enable: true, Status: "pending"}
	database.GetDB().Create(node)
	var svc InboundService

	junk := filepath.Join(t.TempDir(), "junk.db")
	if err := os.WriteFile(junk, []byte("this is not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(junk)
	defer f.Close()
	if _, err := svc.ImportXUIDB(f, node.Id, 1); err == nil {
		t.Fatal("a non-database file must be rejected")
	}

	// A valid SQLite file that is not an x-ui.db.
	other := filepath.Join(t.TempDir(), "other.db")
	db, _ := gorm.Open(sqlite.Open(other), &gorm.Config{Logger: logger.Discard})
	db.Exec("CREATE TABLE things (id integer)")
	sqlDB, _ := db.DB()
	_ = sqlDB.Close()
	g, _ := os.Open(other)
	defer g.Close()
	if _, err := svc.ImportXUIDB(g, node.Id, 1); err == nil {
		t.Fatal("a database without inbounds must be rejected")
	}

	if _, err := svc.ImportXUIDB(g, 9999, 1); err == nil {
		t.Fatal("an unknown node must be rejected")
	}
}
