package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xraygeodata "github.com/xtls/xray-core/common/geodata"
	"google.golang.org/protobuf/proto"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func templateWithOutbound(t *testing.T, tag string) string {
	t.Helper()
	raw, err := (&SettingService{}).GetXrayConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	outbounds, _ := m["outbounds"].([]any)
	m["outbounds"] = append(outbounds, map[string]any{"protocol": "freedom", "tag": tag, "settings": map[string]any{}})
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestNodeTemplateOverridesTheGlobalOne(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	writeMinimalGeoip(t)
	n1 := &model.Node{Name: "n1", Address: "n1.example.com", Enable: true}
	n2 := &model.Node{Name: "n2", Address: "n2.example.com", Enable: true}
	for _, n := range []*model.Node{n1, n2} {
		if err := database.GetDB().Create(n).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc, xs := &SettingService{}, &XraySettingService{}
	x := &XrayService{}

	has := func(nodeID int, tag string) bool {
		cfg, err := x.GetXrayConfigForNode(nodeID)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(string(cfg), `"tag":"`+tag+`"`)
	}

	if has(n1.Id, "only-n1") {
		t.Fatal("marker present before the override exists")
	}
	if _, over, err := svc.GetXrayConfigTemplateForNode(n1.Id); err != nil || over {
		t.Fatalf("no override expected: %v %v", over, err)
	}

	if err := xs.SaveNodeXraySetting(n1.Id, templateWithOutbound(t, "only-n1")); err != nil {
		t.Fatal(err)
	}
	if !has(n1.Id, "only-n1") {
		t.Fatal("node 1 must use its own template")
	}
	if has(n2.Id, "only-n1") {
		t.Fatal("node 2 must keep the global template")
	}
	if _, over, _ := svc.GetXrayConfigTemplateForNode(n1.Id); !over {
		t.Fatal("override flag not reported")
	}

	// An invalid template is refused and the existing override stays.
	if err := xs.SaveNodeXraySetting(n1.Id, `{"outbounds": "nope"}`); err == nil {
		t.Fatal("invalid template accepted")
	}
	if !has(n1.Id, "only-n1") {
		t.Fatal("a refused save must not change the override")
	}
	if err := xs.SaveNodeXraySetting(999, templateWithOutbound(t, "x")); err == nil {
		t.Fatal("unknown node accepted")
	}

	if err := xs.DeleteNodeXraySetting(n1.Id); err != nil {
		t.Fatal(err)
	}
	if has(n1.Id, "only-n1") {
		t.Fatal("deleting the override must restore the global template")
	}

	// Deleting the node removes its override.
	if err := xs.SaveNodeXraySetting(n2.Id, templateWithOutbound(t, "only-n2")); err != nil {
		t.Fatal(err)
	}
	if err := (&NodeService{}).Delete(n2.Id); err != nil {
		t.Fatal(err)
	}
	var left int64
	database.GetDB().Model(&model.NodeXrayTemplate{}).Where("node_id = ?", n2.Id).Count(&left)
	if left != 0 {
		t.Fatal("override outlived its node")
	}
}

// writeMinimalGeoip gives the Xray config validator the one geo list the default template references.
func writeMinimalGeoip(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	data, err := proto.Marshal(&xraygeodata.GeoIPList{Entry: []*xraygeodata.GeoIP{
		{Code: "PRIVATE", Cidr: []*xraygeodata.CIDR{{Ip: []byte{10, 0, 0, 0}, Prefix: 8}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geoip.dat"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	// The core resolves its asset folder once per process, possibly before this test ran, so also
	// provide the list at the default relative location.
	if _, err := os.Stat("bin"); os.IsNotExist(err) {
		if err := os.MkdirAll("bin", 0o755); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll("bin") })
		if err := os.WriteFile(filepath.Join("bin", "geoip.dat"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
