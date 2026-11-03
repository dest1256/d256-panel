package service

import (
	"encoding/json"
	"testing"

	wgutil "github.com/mhsanaei/3x-ui/v3/internal/util/wireguard"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func wgKeypairForTest() (priv, pub string, err error) {
	return wgutil.GenerateWireguardKeypair()
}

func makeAWGOutboundConfig(t *testing.T) *xray.Config {
	t.Helper()
	cfg := &xray.Config{}
	err := json.Unmarshal([]byte(`{
		"outbounds": [
			{"protocol": "freedom", "tag": "direct"},
			{"protocol": "amneziawg", "tag": "awg-hop", "settings": {"secretKey": "x"}}
		]
	}`), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestCheckXrayConfig_RejectsBrokenAWGOutbound(t *testing.T) {
	// The emptied field's partner must be a real key, or the case is decided
	// by that partner and stays green with the empty-key guard removed.
	priv, pub, err := wgKeypairForTest()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		template string
	}{
		{
			name: "not a key",
			template: `{
				"outbounds": [{
					"protocol": "amneziawg",
					"tag": "awg-bad",
					"settings": {
						"secretKey": "not-a-key",
						"address": ["10.8.0.2/32"],
						"peers": [{"publicKey": "alsobad", "allowedIPs": ["0.0.0.0/0"], "endpoint": "203.0.113.7:51820"}]
					}
				}]
			}`,
		},
		{
			name: "empty secretKey",
			template: `{
				"outbounds": [{
					"protocol": "amneziawg",
					"tag": "awg-empty-sec",
					"settings": {
						"secretKey": "",
						"address": ["10.8.0.2/32"],
						"peers": [{"publicKey": "` + pub + `", "allowedIPs": ["0.0.0.0/0"], "endpoint": "203.0.113.7:51820"}]
					}
				}]
			}`,
		},
		{
			name: "empty peer publicKey",
			template: `{
				"outbounds": [{
					"protocol": "amneziawg",
					"tag": "awg-empty-pub",
					"settings": {
						"secretKey": "` + priv + `",
						"address": ["10.8.0.2/32"],
						"peers": [{"publicKey": "", "allowedIPs": ["0.0.0.0/0"], "endpoint": "203.0.113.7:51820"}]
					}
				}]
			}`,
		},
	}
	svc := &XraySettingService{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.CheckXrayConfig(tc.template); err == nil {
				t.Fatalf("%s: expected error, got nil", tc.name)
			}
		})
	}
}

func TestCheckXrayConfig_RejectsEmptyTagAWGOutbound(t *testing.T) {
	priv, pub, err := wgKeypairForTest()
	if err != nil {
		t.Fatal(err)
	}
	template := `{
		"outbounds": [{
			"protocol": "amneziawg",
			"tag": "",
			"settings": {
				"secretKey": "` + priv + `",
				"address": ["10.8.0.2/32"],
				"peers": [{"publicKey": "` + pub + `", "allowedIPs": ["0.0.0.0/0"], "endpoint": "203.0.113.7:51820"}]
			}
		}]
	}`
	svc := &XraySettingService{}
	if err := svc.CheckXrayConfig(template); err == nil {
		t.Fatal("empty-tag amneziawg outbound accepted by CheckXrayConfig")
	}
}

func TestCheckXrayConfig_RejectsNonStringTagAWGOutbound(t *testing.T) {
	template := `{
		"outbounds": [{
			"protocol": "amneziawg",
			"tag": 123,
			"settings": {"secretKey": "x"}
		}]
	}`
	svc := &XraySettingService{}
	if err := svc.CheckXrayConfig(template); err == nil {
		t.Fatal("non-string tag amneziawg outbound accepted by CheckXrayConfig")
	}
}
