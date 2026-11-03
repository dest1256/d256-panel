package service

import (
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	wgutil "github.com/mhsanaei/3x-ui/v3/internal/util/wireguard"
)

// A real X25519 pair, so PublicKeyFromPrivate agrees with the stored value.
var awgTestPrivateKey, awgTestPublicKey = func() (string, string) {
	priv, pub, err := wgutil.GenerateWireguardKeypair()
	if err != nil {
		panic(err)
	}
	return priv, pub
}()

func TestCheckForwardedPortsConflict_EmptySpecNoConflict(t *testing.T) {
	setupConflictDB(t)
	svc := &InboundService{}
	ctx, err := svc.loadPortConflictContext(database.GetDB(), nil)
	if err != nil {
		t.Fatalf("loadPortConflictContext: %v", err)
	}
	if hit := svc.checkForwardedPortsConflict(ctx, ""); hit != "" {
		t.Fatalf("an empty spec must never conflict; got hit=%q", hit)
	}
}

func TestCheckForwardedPortsConflict_IgnoresDisabledInboundPort(t *testing.T) {
	setupConflictDB(t)
	disabled := &model.Inbound{Tag: "vless-8080-off", Enable: false, Listen: "0.0.0.0", Port: 8080, Protocol: model.VLESS, StreamSettings: `{"network":"tcp"}`}
	if err := database.GetDB().Create(disabled).Error; err != nil {
		t.Fatalf("seed disabled inbound: %v", err)
	}

	svc := &InboundService{}
	ctx, err := svc.loadPortConflictContext(database.GetDB(), nil)
	if err != nil {
		t.Fatalf("loadPortConflictContext: %v", err)
	}
	if hit := svc.checkForwardedPortsConflict(ctx, "8080"); hit != "" {
		t.Fatalf("a disabled inbound's port must not be reserved; got hit=%q", hit)
	}
}

func TestCheckForwardedPortsConflict_NoCollisionWhenPortsDontOverlap(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflict(t, "vless-8080", "0.0.0.0", 8080, model.VLESS, `{"network":"tcp"}`, `{}`)

	svc := &InboundService{}
	ctx, err := svc.loadPortConflictContext(database.GetDB(), nil)
	if err != nil {
		t.Fatalf("loadPortConflictContext: %v", err)
	}
	if hit := svc.checkForwardedPortsConflict(ctx, "9075-9085"); hit != "" {
		t.Fatalf("unrelated ports must not conflict; got hit=%q", hit)
	}
}

// A port-forward spec matching a port used only by an inbound hosted on a
// DIFFERENT node must not conflict: that inbound's DNAT/listen socket lives
// on the node's own host, never on this panel's, so there is nothing here
// for the forwarded port to actually collide with. Mirrors
// TestCheckPortConflict_NodeScope's own reasoning for the general port-
// conflict check.
func TestCheckForwardedPortsConflict_IgnoresPortOnDifferentNode(t *testing.T) {
	setupConflictDB(t)
	seedInboundConflictNode(t, "node1-8080", "0.0.0.0", 8080, model.VLESS, `{"network":"tcp"}`, `{}`, new(1))

	svc := &InboundService{}
	ctx, err := svc.loadPortConflictContext(database.GetDB(), nil)
	if err != nil {
		t.Fatalf("loadPortConflictContext: %v", err)
	}
	if hit := svc.checkForwardedPortsConflict(ctx, "8080"); hit != "" {
		t.Fatalf("a port used only on a different node must not conflict; got hit=%q", hit)
	}
}

// A protocol switch keeps the stored clients, re-injected after the AmneziaWG
// normalize ran, so a WireGuard row's bad allowedIPs skipped every check.
func TestUpdateInbound_ProtocolSwitchRejectsStoredClientAllowedIPs(t *testing.T) {
	setupConflictDB(t)
	stored := mkInbound(t, 51830, model.WireGuard, `{"secretKey":"`+awgTestPrivateKey+`","clients":[{"email":"sw@x","enable":true,`+
		`"privateKey":"`+awgTestPrivateKey+`","publicKey":"`+awgTestPublicKey+`","allowedIPs":["10.8.1.2/32\n[Interface]\nPostUp = touch /tmp/pwned"]}]}`)

	update := *stored
	update.Protocol = model.AmneziaWG
	update.Settings = `{"server":{"privateKey":"x","publicKey":"y","subnetIp":"10.8.1.0","subnetCidr":24},"clients":[]}`
	_, _, err := (&InboundService{}).UpdateInbound(&update)
	if err == nil || !strings.Contains(err.Error(), "invalid allowedIPs entry") {
		t.Fatalf("UpdateInbound err = %v, want an invalid allowedIPs entry error", err)
	}

	var after model.Inbound
	if err := database.GetDB().First(&after, stored.Id).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if after.Protocol != model.WireGuard {
		t.Fatalf("rejected switch was persisted: protocol = %q", after.Protocol)
	}
}
