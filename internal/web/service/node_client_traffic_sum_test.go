package service

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
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

// createNodeInboundWithClient mirrors createNodeInbound but stores the client
// in the settings JSON, which the node sync turns into a client_inbounds link.
func createNodeInboundWithClient(t *testing.T, db *gorm.DB, nodeID int, tag string, port int, email string) {
	t.Helper()
	nid := nodeID
	settings := fmt.Sprintf(`{"clients": [{"email": %q, "enable": true}]}`, email)
	ib := &model.Inbound{UserId: 1, Tag: tag, Enable: true, Port: port, Protocol: model.VLESS, NodeID: &nid, Settings: settings}
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

func assertUpDown(t *testing.T, ct xray.ClientTraffic, wantUp, wantDown int64, when string) {
	t.Helper()
	if ct.Up != wantUp || ct.Down != wantDown {
		t.Errorf("%s: up=%d down=%d, want %d/%d", when, ct.Up, ct.Down, wantUp, wantDown)
	}
}

var clientWriteSQL = regexp.MustCompile(`(?i)^\s*(?:INSERT\s+INTO|UPDATE|DELETE\s+FROM)\s+["` + "`" + `]?(clients|client_inbounds|client_traffics|node_client_traffics)\b`)

// countClientRowWrites returns, per client table, how many rows fn's
// INSERT/UPDATE/DELETE statements touched.
func countClientRowWrites(t *testing.T, fn func()) map[string]int64 {
	t.Helper()
	db := database.GetDB()
	var mu sync.Mutex
	rows := map[string]int64{}
	record := func(tx *gorm.DB) {
		if tx.Error != nil || tx.RowsAffected == 0 || tx.Statement == nil {
			return
		}
		if m := clientWriteSQL.FindStringSubmatch(tx.Statement.SQL.String()); m != nil {
			mu.Lock()
			rows[m[1]] += tx.RowsAffected
			mu.Unlock()
		}
	}
	const name = "test:count_client_row_writes"
	cb := db.Callback()
	for _, err := range []error{
		cb.Create().After("gorm:create").Register(name, record),
		cb.Update().After("gorm:update").Register(name, record),
		cb.Delete().After("gorm:delete").Register(name, record),
		cb.Raw().After("gorm:raw").Register(name, record),
	} {
		if err != nil {
			t.Fatalf("register write callback: %v", err)
		}
	}
	defer func() {
		for _, err := range []error{
			cb.Create().Remove(name), cb.Update().Remove(name),
			cb.Delete().Remove(name), cb.Raw().Remove(name),
		} {
			if err != nil {
				t.Errorf("remove write callback: %v", err)
			}
		}
	}()
	fn()
	return rows
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
