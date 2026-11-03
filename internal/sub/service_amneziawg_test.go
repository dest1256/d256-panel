package sub

import (
	"fmt"
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Regression test for the bug where getInboundsBySubId's SQL allowlist was
// missing 'amneziawg', silently excluding every AmneziaWG client from
// subscriptions (plain/individual links, JSON, Clash) even though
// genAmneziaWGLink itself was already fully implemented and wired into
// GetLink's dispatch switch.
func TestGetInboundsBySubIdIncludesAmneziaWG(t *testing.T) {
	initSubDB(t)
	db := database.GetDB()

	in := &model.Inbound{Port: 51820, Protocol: model.AmneziaWG, Enable: true, Tag: "awg-sub", Settings: `{"server":{"privateKey":"x","publicKey":"y"},"clients":[]}`}
	if err := db.Create(in).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	rec := &model.ClientRecord{Email: "u@awg", SubID: "subawg", Enable: true}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: rec.Id, InboundId: in.Id}).Error; err != nil {
		t.Fatalf("create link: %v", err)
	}

	s := &SubService{}
	inbounds, err := s.getInboundsBySubId("subawg")
	if err != nil {
		t.Fatalf("getInboundsBySubId: %v", err)
	}
	if len(inbounds) != 1 || inbounds[0].Id != in.Id {
		t.Fatalf("amneziawg inbound not returned for subId: %+v", inbounds)
	}
}

// Membership and account metadata stay on the normalized row. Settings may carry a
// stale subId/enable and an extra email; tunnel fields still come from this inbound (#6641).
func TestMatchingClients_TunnelMetadataStaysNormalized(t *testing.T) {
	const (
		subID       = "sub-meta"
		email       = "user@awg"
		freshID     = "11111111-2222-4333-8444-555555555555"
		staleID     = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
		settingsKey = "settings-private-key"
		sharedKey   = "shared-private-key"
		expiry      = int64(1700000000000)
	)
	clientsJSON := fmt.Sprintf(`[
		{"id":%q,"email":"User@AWG","subId":"stale-sub","enable":false,"totalGB":1,"expiryTime":1,"comment":"stale","limitIp":9,"privateKey":%q,"publicKey":"settings-pub","allowedIPs":["10.8.1.2/32","fd00::2/128"],"preSharedKey":"settings-psk","keepAlive":15},
		{"email":"settings-only@awg","subId":%q,"enable":true,"privateKey":"only-priv","allowedIPs":["10.8.1.9/32"]}
	]`, staleID, settingsKey, subID)

	for _, protocol := range []model.Protocol{model.AmneziaWG, model.WireGuard} {
		t.Run(string(protocol), func(t *testing.T) {
			initSubDB(t)
			db := database.GetDB()
			settings := `{"secretKey":"c2VydmVy","clients":` + clientsJSON + `}`
			if protocol == model.AmneziaWG {
				settings = `{"server":{"privateKey":"c2VydmVy","publicKey":"cHVi"},"clients":` + clientsJSON + `}`
			}
			ib := &model.Inbound{
				UserId: 1, Tag: "meta-" + string(protocol), Enable: true, Listen: "203.0.113.50", Port: 51850,
				Protocol: protocol, Remark: "meta", Settings: settings,
			}
			if err := db.Create(ib).Error; err != nil {
				t.Fatalf("create inbound: %v", err)
			}
			rec := &model.ClientRecord{
				Email: email, SubID: subID, UUID: freshID, Enable: true,
				TotalGB: 5, ExpiryTime: expiry, Comment: "vip", LimitIP: 3,
				PrivateKey: sharedKey, PublicKey: "shared-pub", AllowedIPs: "10.9.9.9/32",
				PreSharedKey: "shared-psk", KeepAlive: model.KeepAliveValue("99"),
			}
			other := &model.ClientRecord{
				Email: "other-sub@awg", SubID: "other-sub", UUID: "22222222-2222-4333-8444-555555555555", Enable: true,
			}
			for _, row := range []*model.ClientRecord{rec, other} {
				if err := db.Create(row).Error; err != nil {
					t.Fatalf("create client %s: %v", row.Email, err)
				}
				if err := db.Create(&model.ClientInbound{ClientId: row.Id, InboundId: ib.Id}).Error; err != nil {
					t.Fatalf("link %s: %v", row.Email, err)
				}
			}

			s := &SubService{}
			got := s.matchingClients(ib, subID)
			if len(got) != 1 {
				t.Fatalf("clients = %d, want the one normalized member: %+v", len(got), got)
			}
			c := got[0]
			if c.Email != email || c.ID != freshID || c.SubID != subID || !c.Enable || c.TotalGB != 5 || c.ExpiryTime != expiry || c.Comment != "vip" || c.LimitIP != 3 {
				t.Fatalf("normalized metadata = %+v", c)
			}
			if c.PrivateKey != settingsKey || c.PublicKey != "settings-pub" || c.PreSharedKey != "settings-psk" || c.KeepAliveSeconds() != 15 {
				t.Fatalf("tunnel identity = key %q pub %q psk %q ka %d", c.PrivateKey, c.PublicKey, c.PreSharedKey, c.KeepAliveSeconds())
			}
			if !slices.Equal(c.AllowedIPs, []string{"10.8.1.2/32", "fd00::2/128"}) {
				t.Fatalf("allowedIPs = %v, want this inbound's v4 and v6", c.AllowedIPs)
			}
			cached, ok := s.clientForLink(ib, email)
			if !ok || cached.PrivateKey != settingsKey || cached.PreSharedKey != "settings-psk" || cached.KeepAliveSeconds() != 15 || !slices.Equal(cached.AllowedIPs, c.AllowedIPs) {
				t.Fatalf("primed cache = %+v, ok %v", cached, ok)
			}
			if extra := s.matchingClients(ib, "nope"); len(extra) != 0 {
				t.Fatalf("non-matching subId must yield 0 clients, got %d", len(extra))
			}
		})
	}
}
