package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	wgutil "github.com/mhsanaei/3x-ui/v3/internal/util/wireguard"
)

// awgRelayWindowSettings builds an AmneziaWG settings blob AddInbound accepts:
// real X25519 keys, one enabled peer, and an email unique to tag.
func awgRelayWindowSettings(t *testing.T, tag string) string {
	t.Helper()
	_, clientPub, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		t.Fatalf("generate client keypair: %v", err)
	}
	return `{"server":{"privateKey":"` + awgTestPrivateKey + `","publicKey":"` + awgTestPublicKey +
		`","subnetIp":"10.8.1.0","subnetCidr":24},"clients":[{"email":"` + tag + `@relay-window","enable":true,"publicKey":"` +
		clientPub + `","allowedIPs":["10.8.1.2/32"]}]}`
}

// awgRelayWindowSettingsWithForward is awgRelayWindowSettings with one client's
// forwardedPorts set, the field the create-time guard validates.
func awgRelayWindowSettingsWithForward(t *testing.T, tag, forwardedPorts string) string {
	t.Helper()
	settings := awgRelayWindowSettings(t, tag)
	return strings.Replace(settings, `"enable":true`, `"enable":true,"forwardedPorts":"`+forwardedPorts+`"`, 1)
}

// pushInboundIDSequence makes the next inbounds insert land on nextID, standing
// in for a long-lived database whose AUTOINCREMENT counter has climbed there.
func pushInboundIDSequence(t *testing.T, nextID int) {
	t.Helper()
	// The counter is a sqlite_sequence row, so this has no PostgreSQL equivalent.
	if database.IsPostgres() {
		t.Skip("the inbounds AUTOINCREMENT counter is a SQLite row")
	}
	res := database.GetDB().Exec("UPDATE sqlite_sequence SET seq = ? WHERE name = ?", nextID-1, "inbounds")
	if res.Error != nil {
		t.Fatalf("push the inbounds sequence to %d: %v", nextID, res.Error)
	}
	if res.RowsAffected != 1 {
		t.Fatalf("inbounds has no AUTOINCREMENT counter row to push (%d rows updated)", res.RowsAffected)
	}
}

func addAmneziaWGInbound(t *testing.T, tag string, port int, enable bool) *model.Inbound {
	t.Helper()
	created, _, err := (&InboundService{}).AddInbound(&model.Inbound{
		Tag:      tag,
		Enable:   enable,
		Listen:   "0.0.0.0",
		Port:     port,
		Protocol: model.AmneziaWG,
		Settings: awgRelayWindowSettings(t, tag),
	})
	if err != nil {
		t.Fatalf("AddInbound(%s): %v", tag, err)
	}
	return created
}
