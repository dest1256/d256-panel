package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func seedVersionedNode(t *testing.T, panelVersion string) *model.Node {
	t.Helper()
	node := &model.Node{
		Name: "n-" + panelVersion, Address: "127.0.0.1", Port: 2096, Scheme: "https",
		Enable: true, Status: "online", PanelVersion: panelVersion,
	}
	seedNodeRow(t, database.GetDB(), node)
	return node
}

func sidecarNodeInbound(t *testing.T, protocol model.Protocol, tag string, nodeID int) *model.Inbound {
	t.Helper()
	ib := &model.Inbound{Tag: tag, Enable: true, Listen: "0.0.0.0", Port: 44300, Protocol: protocol, NodeID: &nodeID}
	switch protocol {
	case model.AmneziaWG:
		ib.Settings = awgRelayWindowSettings(t, tag)
	case model.TUIC:
		ib.Settings = `{"clients":[{"id":"8a4f0c7e-1d2b-4c3a-9e5f-6a7b8c9d0e1f","password":"pass","email":"` + tag + `@tuic","enable":true}]}`
	case model.MTProto:
		ib.Settings = `{"clients":[{"email":"` + tag + `@mt","enable":true,"secret":"ee0123456789abcdef0123456789abcdef"}]}`
	default:
		t.Fatalf("no fixture for %s", protocol)
	}
	return ib
}

// The node's own panel runs the sidecar for a pushed row, so a sidecar protocol
// is deployable to any node new enough to know it -- the release that added it.
func TestAddInbound_SidecarProtocolDeploysToANodeAtItsFirstRelease(t *testing.T) {
	cases := []struct {
		protocol     model.Protocol
		firstRelease string
	}{
		{model.MTProto, "v3.5.0"},
		{model.AmneziaWG, "v3.7.0"},
		{model.TUIC, "v3.8.0"},
	}
	for _, tc := range cases {
		t.Run(string(tc.protocol), func(t *testing.T) {
			setupConflictDB(t)
			node := seedVersionedNode(t, tc.firstRelease)

			created, _, err := (&InboundService{}).AddInbound(sidecarNodeInbound(t, tc.protocol, "side-"+string(tc.protocol), node.Id))
			if err != nil {
				t.Fatalf("AddInbound(%s on a %s node): %v", tc.protocol, tc.firstRelease, err)
			}
			var stored model.Inbound
			if err := database.GetDB().First(&stored, created.Id).Error; err != nil {
				t.Fatalf("read created row: %v", err)
			}
			if stored.NodeID == nil || *stored.NodeID != node.Id {
				t.Fatalf("stored nodeId = %v, want %d", stored.NodeID, node.Id)
			}
		})
	}
}

// A node that predates a protocol stores it as an Xray inbound Xray cannot load,
// so the assignment is refused while the version is too old or still unknown.
func TestAddInbound_SidecarProtocolNodeVersionGate(t *testing.T) {
	cases := []struct {
		name         string
		panelVersion string
		wantErr      string
	}{
		{"one release too old", "v3.6.9", "v3.7.0"},
		{"version not reported yet", "", "has not reported its panel version"},
		{"dev build", "dev+1d1128cf", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupConflictDB(t)
			node := seedVersionedNode(t, tc.panelVersion)

			_, _, err := (&InboundService{}).AddInbound(sidecarNodeInbound(t, model.AmneziaWG, "awg-gate", node.Id))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("a dev build tracks main and knows every protocol; got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want a refusal mentioning %q", err, tc.wantErr)
			}
		})
	}
}

// awgClientsPayload is the clients-only body the client endpoints take: one
// fresh peer of tag at address, forwarding forwardedPorts.
func awgClientsPayload(t *testing.T, tag, address, forwardedPorts string) string {
	t.Helper()
	settings := replaceFirst(t, awgRelayWindowSettingsWithForward(t, tag, forwardedPorts),
		`"allowedIPs":["10.8.1.2/32"]`, `"allowedIPs":["`+address+`"]`)
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(settings), &parsed); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return `{"clients":` + string(parsed["clients"]) + `}`
}

// seedNodeAmneziaWGBesideLocalHolder seeds a node AmneziaWG row and a local
// inbound on 8443, the port a forward on the node is free to take.
func seedNodeAmneziaWGBesideLocalHolder(t *testing.T) *model.Inbound {
	t.Helper()
	setupConflictDB(t)
	node := seedVersionedNode(t, "v3.8.0")
	seedInboundConflict(t, "holder", "0.0.0.0", 8443, model.VLESS, `{"network":"tcp"}`, `{"clients":[]}`)
	seedInboundConflictNode(t, "awg-node", "0.0.0.0", 51820, model.AmneziaWG, ``, awgRelayWindowSettings(t, "awg-node"), &node.Id)
	var row model.Inbound
	if err := database.GetDB().Where("tag = ?", "awg-node").First(&row).Error; err != nil {
		t.Fatalf("read seeded row: %v", err)
	}
	return &row
}

// The edit payload carries no reliable nodeId, so the forward check must use
// the stored one or it judges a node row against the master's own ports.
func TestUpdateInbound_NodeAmneziaWGForwardIgnoresMasterPorts(t *testing.T) {
	row := seedNodeAmneziaWGBesideLocalHolder(t)

	update := *row
	update.NodeID = nil
	update.Settings = replaceFirst(t, row.Settings, `"enable":true`, `"enable":true,"forwardedPorts":"8443"`)
	if _, _, err := (&InboundService{}).UpdateInbound(&update); err != nil {
		t.Fatalf("8443 is bound on the master, not on the node; the edit must be allowed: %v", err)
	}
}

func TestAddInboundClient_NodeAmneziaWGForwardIgnoresMasterPorts(t *testing.T) {
	row := seedNodeAmneziaWGBesideLocalHolder(t)

	data := &model.Inbound{Id: row.Id, Settings: awgClientsPayload(t, "awg-new", "10.8.1.3/32", "8443")}
	if _, err := (&ClientService{}).AddInboundClient(&InboundService{}, data); err != nil {
		t.Fatalf("8443 is bound on the master, not on the node; the add must be allowed: %v", err)
	}
}

func TestUpdateInboundClient_NodeAmneziaWGForwardIgnoresMasterPorts(t *testing.T) {
	row := seedNodeAmneziaWGBesideLocalHolder(t)

	var stored struct {
		Clients []json.RawMessage `json:"clients"`
	}
	if err := json.Unmarshal([]byte(row.Settings), &stored); err != nil {
		t.Fatalf("decode seeded settings: %v", err)
	}
	edited := replaceFirst(t, string(stored.Clients[0]), `"enable":true`, `"enable":true,"forwardedPorts":"8443"`)
	data := &model.Inbound{Id: row.Id, Settings: `{"clients":[` + edited + `]}`}
	if _, err := (&ClientService{}).UpdateInboundClient(&InboundService{}, data, "awg-node@relay-window"); err != nil {
		t.Fatalf("8443 is bound on the master, not on the node; the edit must be allowed: %v", err)
	}
}
