package service

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestEnsureAPIServices(t *testing.T) {
	// legacy template without RoutingService gets it injected
	out := ensureAPIServices(json_util.RawMessage(`{"services":["HandlerService","LoggerService","StatsService"],"tag":"api"}`))
	var parsed struct {
		Services []string `json:"services"`
		Tag      string   `json:"tag"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"HandlerService": true, "StatsService": true, "RoutingService": true, "LoggerService": true}
	if len(parsed.Services) != 4 {
		t.Fatalf("expected 4 services, got %v", parsed.Services)
	}
	for _, svc := range parsed.Services {
		if !want[svc] {
			t.Fatalf("unexpected service %q", svc)
		}
	}
	if parsed.Tag != "api" {
		t.Fatalf("tag must be preserved, got %q", parsed.Tag)
	}

	// complete api block is returned unchanged (no marshal churn)
	full := json_util.RawMessage(`{"services":["HandlerService","StatsService","RoutingService"],"tag":"api"}`)
	if got := ensureAPIServices(full); string(got) != string(full) {
		t.Fatalf("complete api block must pass through untouched, got %s", got)
	}

	// absent api block stays absent
	if got := ensureAPIServices(nil); got != nil {
		t.Fatalf("nil api block must stay nil, got %s", got)
	}
}

func TestEnsureStatsPolicy(t *testing.T) {
	// default-template shape: level "0" exists with traffic flags — the online
	// flag is added and the siblings survive untouched
	out := ensureStatsPolicy(json_util.RawMessage(`{"levels":{"0":{"handshake":4,"statsUserUplink":true,"statsUserDownlink":true}},"system":{"statsInboundDownlink":true}}`))
	var parsed struct {
		Levels map[string]map[string]any `json:"levels"`
		System map[string]any            `json:"system"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	level0 := parsed.Levels["0"]
	if level0["statsUserOnline"] != true {
		t.Fatalf("statsUserOnline must be injected into level 0, got %v", level0)
	}
	if level0["statsUserUplink"] != true || level0["statsUserDownlink"] != true || level0["handshake"] != float64(4) {
		t.Fatalf("sibling keys must be preserved, got %v", level0)
	}
	if parsed.System["statsInboundDownlink"] != true {
		t.Fatalf("system block must be preserved, got %v", parsed.System)
	}

	// missing levels block: level "0" is created with the flag
	out = ensureStatsPolicy(json_util.RawMessage(`{"system":{}}`))
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Levels["0"]["statsUserOnline"] != true {
		t.Fatalf("level 0 must be created with statsUserOnline, got %s", out)
	}

	// every level gets the flag, an explicit false included — the flag is
	// panel infrastructure, like the api services
	out = ensureStatsPolicy(json_util.RawMessage(`{"levels":{"0":{"statsUserOnline":false},"1":{"connIdle":300}}}`))
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"0", "1"} {
		if parsed.Levels[key]["statsUserOnline"] != true {
			t.Fatalf("level %s must have statsUserOnline forced on, got %s", key, out)
		}
	}
	if parsed.Levels["1"]["connIdle"] != float64(300) {
		t.Fatalf("level 1 siblings must be preserved, got %s", out)
	}

	// already-enabled input passes through byte-identical (no marshal churn,
	// no spurious restart)
	full := json_util.RawMessage(`{"levels":{"0":{"statsUserOnline":true}}}`)
	if got := ensureStatsPolicy(full); string(got) != string(full) {
		t.Fatalf("already-enabled policy must pass through untouched, got %s", got)
	}

	// absent policy block stays absent
	if got := ensureStatsPolicy(nil); got != nil {
		t.Fatalf("nil policy must stay nil, got %s", got)
	}

	// unparsable policy is left untouched
	bad := json_util.RawMessage(`{not json`)
	if got := ensureStatsPolicy(bad); string(got) != string(bad) {
		t.Fatalf("unparsable policy must be left untouched, got %s", got)
	}
}

func egressTestConfig() *xray.Config {
	return &xray.Config{
		RouterConfig:    json_util.RawMessage(`{"domainStrategy":"AsIs","rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"}]}`),
		OutboundConfigs: json_util.RawMessage(`[{"protocol":"freedom","tag":"direct"},{"protocol":"socks","tag":"warp"}]`),
		InboundConfigs: []xray.InboundConfig{
			{Port: 62789, Protocol: "tunnel", Tag: "api", Listen: json_util.RawMessage(`"127.0.0.1"`)},
		},
	}
}

func mtprotoInbound(tag string, settings string) *model.Inbound {
	return &model.Inbound{Tag: tag, Protocol: model.MTProto, Enable: true, Settings: settings}
}

func TestInjectMtprotoEgress_Disabled(t *testing.T) {
	// Not routed, and routed-but-portless, are both no-ops.
	for _, settings := range []string{
		`{"routeThroughXray":false,"routeXrayPort":50000}`,
		`{"routeThroughXray":true}`,
		`{"routeThroughXray":true,"routeXrayPort":0}`,
	} {
		cfg := egressTestConfig()
		before := string(cfg.RouterConfig)
		injectMtprotoEgress(cfg, mtprotoInbound("inbound-443", settings))
		if len(cfg.InboundConfigs) != 1 || string(cfg.RouterConfig) != before {
			t.Fatalf("settings %s must be a no-op, got %d inbounds", settings, len(cfg.InboundConfigs))
		}
	}
}

func TestInjectMtprotoEgress_TagCollisionSkips(t *testing.T) {
	cfg := egressTestConfig()
	cfg.InboundConfigs = append(cfg.InboundConfigs,
		xray.InboundConfig{Port: 443, Protocol: "vless", Tag: "inbound-443"})
	before := string(cfg.RouterConfig)
	injectMtprotoEgress(cfg, mtprotoInbound("inbound-443",
		`{"routeThroughXray":true,"routeXrayPort":50003,"outboundTag":"warp"}`))
	if len(cfg.InboundConfigs) != 2 || string(cfg.RouterConfig) != before {
		t.Fatal("a real inbound already owning the tag must make the bridge a no-op")
	}
}

func TestInjectMtprotoEgress_MissingTargetSkips(t *testing.T) {
	cfg := egressTestConfig()
	before := string(cfg.RouterConfig)
	injectMtprotoEgress(cfg, mtprotoInbound("inbound-443",
		`{"routeThroughXray":true,"routeXrayPort":50004,"outboundTag":"removed-subscription-outbound"}`))

	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("a missing target must not expose the mtproto bridge, got %+v", cfg.InboundConfigs)
	}
	if string(cfg.RouterConfig) != before {
		t.Fatalf("a missing target must leave routing untouched, got %s", cfg.RouterConfig)
	}
}

func TestInjectMtprotoEgress_BadOutboundsSkips(t *testing.T) {
	cfg := egressTestConfig()
	cfg.OutboundConfigs = json_util.RawMessage(`{not json`)
	before := string(cfg.RouterConfig)
	injectMtprotoEgress(cfg, mtprotoInbound("inbound-443",
		`{"routeThroughXray":true,"routeXrayPort":50005,"outboundTag":"direct"}`))

	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("unparsable outbounds must not expose the mtproto bridge, got %+v", cfg.InboundConfigs)
	}
	if string(cfg.RouterConfig) != before {
		t.Fatalf("unparsable outbounds must leave routing untouched, got %s", cfg.RouterConfig)
	}
}

func TestInjectMtprotoEgress_BadRoutingSkips(t *testing.T) {
	cfg := egressTestConfig()
	cfg.RouterConfig = json_util.RawMessage(`{not json`)
	injectMtprotoEgress(cfg, mtprotoInbound("inbound-443",
		`{"routeThroughXray":true,"routeXrayPort":50006,"outboundTag":"direct"}`))

	if len(cfg.InboundConfigs) != 1 {
		t.Fatalf("unparsable routing must not expose the mtproto bridge, got %+v", cfg.InboundConfigs)
	}
	if string(cfg.RouterConfig) != `{not json` {
		t.Fatalf("unparsable routing must be left untouched, got %s", cfg.RouterConfig)
	}
}
