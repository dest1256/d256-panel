package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestUpdateHeartbeatStoresNetMetrics(t *testing.T) {
	_ = setupSettingMtlsDB(t)
	s := &NodeService{}

	n := &model.Node{Name: "netn", Address: "1.2.3.4", Port: 2053, Scheme: "https", ApiToken: "t"}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}

	patch := HeartbeatPatch{Status: "online", LastHeartbeat: time.Now().Unix(), NetUp: 111, NetDown: 222}
	if err := s.UpdateHeartbeat(n.Id, patch); err != nil {
		t.Fatalf("UpdateHeartbeat: %v", err)
	}

	var got model.Node
	if err := database.GetDB().First(&got, n.Id).Error; err != nil {
		t.Fatalf("reload node: %v", err)
	}
	if got.NetUp != 111 || got.NetDown != 222 {
		t.Fatalf("net columns not persisted: up=%d down=%d", got.NetUp, got.NetDown)
	}
	if len(s.AggregateNodeMetric(n.Id, "netUp", 2, 60)) == 0 {
		t.Fatal("expected netUp history points after an online heartbeat")
	}
}

func TestNodeMetricKeysIncludesNet(t *testing.T) {
	for _, k := range []string{"netUp", "netDown"} {
		if !slices.Contains(NodeMetricKeys, k) {
			t.Fatalf("NodeMetricKeys must include %q so the history endpoint accepts it", k)
		}
	}
}

func availabilityNodeFixture(t *testing.T, body string) (*NodeService, *model.Node, func(string)) {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "node.db"))
	var response atomic.Value
	response.Store(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(response.Load().(string)))
	}))
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	n := &model.Node{Name: "sample", Scheme: "http", Address: u.Hostname(), Port: port, BasePath: "/", Enable: true, AllowPrivateAddress: true, TlsVerifyMode: "skip"}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatal(err)
	}
	old := nodeMetrics
	nodeMetrics = newMetricHistory()
	t.Cleanup(func() { nodeMetrics = old })
	return &NodeService{}, n, func(s string) { response.Store(s) }
}

func unavailableMetricsOf(t *testing.T, value any) []string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Unavailable []string `json:"unavailableMetrics"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out.Unavailable
}

const (
	unavailableStatusBody = `{"success":true,"obj":{"cpu":0,"uptime":0,"unavailableMetrics":["cpu","uptime"],"mem":{"current":1,"total":2},"xray":{"state":"running"}}}`
	zeroStatusBody        = `{"success":true,"obj":{"cpu":0,"uptime":0,"mem":{"current":1,"total":2},"xray":{"state":"running"}}}`
)

func TestNodeAvailabilityColumnKeepsLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	dbtest.InitDB(t, path)
	n := &model.Node{Name: "legacy", CpuPct: 0, UptimeSecs: 0}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-change schema, then let the normal startup migration run.
	if err := database.GetDB().Migrator().DropColumn(&model.Node{}, "UnavailableMetrics"); err != nil {
		t.Fatal(err)
	}
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(path); err != nil {
		t.Fatal(err)
	}
	var stored model.Node
	if err := database.GetDB().First(&stored, n.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Name != "legacy" || stored.CpuPct != 0 || stored.UptimeSecs != 0 || len(stored.UnavailableMetrics) != 0 {
		t.Fatalf("legacy row changed: id=%d availability=%v", stored.Id, stored.UnavailableMetrics)
	}
}
