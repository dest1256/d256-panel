package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// nodeUnderContract serves the production router as a node and records every
// request the node refused for auth or scope.
type nodeUnderContract struct {
	srv     *httptest.Server
	mu      sync.Mutex
	refused []string
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (n *nodeUnderContract) takeRefused() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := n.refused
	n.refused = nil
	return out
}

func nodeRow(t *testing.T, tag string) (*model.Inbound, bool) {
	t.Helper()
	var ib model.Inbound
	err := database.GetDB().Where("tag = ?", tag).First(&ib).Error
	return &ib, err == nil
}

func nodeTraffic(t *testing.T, email string) int64 {
	t.Helper()
	var ct xray.ClientTraffic
	if err := database.GetDB().Where("email = ?", email).First(&ct).Error; err != nil {
		t.Fatalf("client_traffics %s: %v", email, err)
	}
	return ct.Up + ct.Down
}

func seedNodeTraffic(t *testing.T, emails ...string) {
	t.Helper()
	for _, e := range emails {
		if err := database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", e).
			Updates(map[string]any{"up": 100, "down": 200}).Error; err != nil {
			t.Fatalf("seed traffic %s: %v", e, err)
		}
	}
	if err := database.GetDB().Model(&model.Inbound{}).Where("tag = ?", contractTag).
		Updates(map[string]any{"up": 100, "down": 200}).Error; err != nil {
		t.Fatalf("seed inbound traffic: %v", err)
	}
}

const contractTag = "in-51001-tcp"

func masterInbound(remark string, enable bool, clients ...string) *model.Inbound {
	entries := make([]string, 0, len(clients))
	for i, email := range clients {
		entries = append(entries, `{"email":"`+email+`","enable":true,"subId":"s-`+email+
			`","id":"0b6d5c2e-7c1a-4f4e-9d3b-00000000000`+strconv.Itoa(i)+`"}`)
	}
	return &model.Inbound{
		Tag: contractTag, Remark: remark, Enable: enable, Port: 51001, Protocol: model.VLESS,
		Settings:       `{"clients":[` + strings.Join(entries, ",") + `],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none","tcpSettings":{"header":{"type":"none"}}}`,
		Sniffing:       `{}`,
	}
}

func nodeEmails(t *testing.T) []string {
	t.Helper()
	ib, ok := nodeRow(t, contractTag)
	if !ok {
		t.Fatal("node has no contract inbound")
	}
	clients, err := (&service.InboundService{}).GetClients(ib)
	if err != nil {
		t.Fatalf("parse node clients: %v", err)
	}
	emails := make([]string, 0, len(clients))
	for _, c := range clients {
		emails = append(emails, c.Email)
	}
	return emails
}

// Remote methods that never reach the node, or that this table must not run.
var remoteMethodsOutsideContract = map[string]string{
	"Name":                  "local label",
	"RecordAdoptedInbound":  "local fingerprint bookkeeping",
	"AdoptInboundAlias":     "local alias bookkeeping",
	"AdoptedInboundAliases": "local alias bookkeeping",
	"AdvancePushedInbound":  "local fingerprint bookkeeping",
	"ForgetPushedInbound":   "local fingerprint bookkeeping",
	"UpdatePanel":           "replaces the node binary; node-sync is denied it on purpose (#6201)",
}
