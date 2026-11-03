package service

import (
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestMtprotoRoutesThroughXray(t *testing.T) {
	cases := map[string]struct {
		ib   *model.Inbound
		want bool
	}{
		"routed":      {&model.Inbound{Protocol: model.MTProto, Settings: `{"routeThroughXray":true}`}, true},
		"off":         {&model.Inbound{Protocol: model.MTProto, Settings: `{"routeThroughXray":false}`}, false},
		"absent":      {&model.Inbound{Protocol: model.MTProto, Settings: `{}`}, false},
		"non-mtproto": {&model.Inbound{Protocol: model.VLESS, Settings: `{"routeThroughXray":true}`}, false},
		"bad json":    {&model.Inbound{Protocol: model.MTProto, Settings: `{nope`}, false},
		"nil":         {nil, false},
	}
	for name, c := range cases {
		if got := mtprotoRoutesThroughXray(c.ib); got != c.want {
			t.Fatalf("%s: got %v want %v", name, got, c.want)
		}
	}
}

func routeXrayPortOf(t *testing.T, settings string) int {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("settings not valid JSON: %v\n%s", err, settings)
	}
	return settingsRouteXrayPort(parsed)
}

func TestFillProtocolDefaultsMtproto(t *testing.T) {
	cs := &ClientService{}
	ib := &model.Inbound{Protocol: model.MTProto, Settings: `{"fakeTlsDomain":"example.com"}`}

	c := &model.Client{Email: "u"}
	if err := cs.fillProtocolDefaults(c, ib); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.Secret, "ee") || !strings.HasSuffix(c.Secret, hex.EncodeToString([]byte("example.com"))) {
		t.Fatalf("mtproto client should get a FakeTLS secret fronting the inbound domain, got %q", c.Secret)
	}

	// An existing secret is not overwritten.
	pre := &model.Client{Email: "v", Secret: "eepreset"}
	if err := cs.fillProtocolDefaults(pre, ib); err != nil {
		t.Fatal(err)
	}
	if pre.Secret != "eepreset" {
		t.Fatalf("an existing secret must be preserved, got %q", pre.Secret)
	}

	// With no inbound domain the default fronting host is used.
	c2 := &model.Client{Email: "w"}
	if err := cs.fillProtocolDefaults(c2, &model.Inbound{Protocol: model.MTProto, Settings: `{}`}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(c2.Secret, hex.EncodeToString([]byte(defaultMtprotoDomain))) {
		t.Fatalf("a domainless inbound should front the default host, got %q", c2.Secret)
	}
}

func TestNormalizeMtprotoSecretHealsClients(t *testing.T) {
	s := &InboundService{}
	ib := &model.Inbound{Protocol: model.MTProto, Settings: `{"fakeTlsDomain":"a.com","secret":"eedeadbeef","clients":[{"email":"x","secret":""}]}`}
	s.normalizeMtprotoSecret(ib)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &parsed); err != nil {
		t.Fatalf("healed settings not valid json: %v", err)
	}
	if _, ok := parsed["secret"]; ok {
		t.Fatalf("the vestigial inbound-level secret must be stripped, got %q", ib.Settings)
	}
	clients := parsed["clients"].([]any)
	got := clients[0].(map[string]any)["secret"].(string)
	if !strings.HasPrefix(got, "ee") || !strings.HasSuffix(got, hex.EncodeToString([]byte("a.com"))) {
		t.Fatalf("client secret should be healed to front the inbound domain, got %q", got)
	}
}
