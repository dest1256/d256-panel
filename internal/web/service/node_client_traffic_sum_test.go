package service

import (
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func initTrafficTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))
	return database.GetDB()
}

func createNodeInbound(t *testing.T, db *gorm.DB, nodeID int, tag string, port int) {
	t.Helper()
	nid := nodeID
	ib := &model.Inbound{UserId: 1, Tag: tag, Enable: true, Port: port, Protocol: model.VLESS, NodeID: &nid}
	if err := db.Create(ib).Error; err != nil {
		t.Fatalf("create node inbound %q: %v", tag, err)
	}
}

func readTraffic(t *testing.T, db *gorm.DB, email string) xray.ClientTraffic {
	t.Helper()
	var ct xray.ClientTraffic
	if err := db.Model(xray.ClientTraffic{}).Where("email = ?", email).First(&ct).Error; err != nil {
		t.Fatalf("read client_traffics %q: %v", email, err)
	}
	return ct
}

func TestDelClientStat_CleansNodeBaselines(t *testing.T) {
	db := initTrafficTestDB(t)
	svc := &InboundService{}

	const email = "gone"
	if err := db.Create(&xray.ClientTraffic{InboundId: 1, Email: email, Enable: true}).Error; err != nil {
		t.Fatalf("seed client_traffics: %v", err)
	}
	if err := db.Create(&model.NodeClientTraffic{NodeId: 1, Email: email, Up: 10, Down: 10}).Error; err != nil {
		t.Fatalf("seed node baseline 1: %v", err)
	}
	if err := db.Create(&model.NodeClientTraffic{NodeId: 2, Email: email, Up: 20, Down: 20}).Error; err != nil {
		t.Fatalf("seed node baseline 2: %v", err)
	}

	if err := svc.DelClientStat(db, email); err != nil {
		t.Fatalf("DelClientStat: %v", err)
	}

	var cnt int64
	if err := db.Model(&model.NodeClientTraffic{}).Where("email = ?", email).Count(&cnt).Error; err != nil {
		t.Fatalf("count baselines: %v", err)
	}
	if cnt != 0 {
		t.Errorf("expected node baselines cleaned, found %d", cnt)
	}
}

func TestNodeDelete_CleansNodeBaselines(t *testing.T) {
	db := initTrafficTestDB(t)
	nodeSvc := NodeService{}

	if err := db.Create(&model.NodeClientTraffic{NodeId: 7, Email: "a", Up: 1, Down: 1}).Error; err != nil {
		t.Fatalf("seed node 7 a: %v", err)
	}
	if err := db.Create(&model.NodeClientTraffic{NodeId: 7, Email: "b", Up: 2, Down: 2}).Error; err != nil {
		t.Fatalf("seed node 7 b: %v", err)
	}
	if err := db.Create(&model.NodeClientTraffic{NodeId: 8, Email: "c", Up: 3, Down: 3}).Error; err != nil {
		t.Fatalf("seed node 8 c: %v", err)
	}

	if err := nodeSvc.Delete(7); err != nil {
		t.Fatalf("NodeService.Delete(7): %v", err)
	}

	var sevenCnt, eightCnt int64
	db.Model(&model.NodeClientTraffic{}).Where("node_id = ?", 7).Count(&sevenCnt)
	db.Model(&model.NodeClientTraffic{}).Where("node_id = ?", 8).Count(&eightCnt)
	if sevenCnt != 0 {
		t.Errorf("node 7 baselines not cleaned: %d remain", sevenCnt)
	}
	if eightCnt != 1 {
		t.Errorf("node 8 baseline should survive, found %d", eightCnt)
	}
}
