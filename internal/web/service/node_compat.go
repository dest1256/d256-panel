package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/web/runtime"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// Nodes are driven by the desired state the node link renders from the database, so the helpers the
// inbound/client services used to push changes to remote panels are now inert. They stay as thin
// shims until their call sites are removed.

const (
	nodeBulkPushThreshold = 32
	nodeFanoutConcurrency = 32
)

func nodePushContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), time.Second)
}

func (s *InboundService) runtimeFor(ib *model.Inbound) (runtime.Runtime, error) {
	mgr := runtime.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("runtime manager not initialised")
	}
	return mgr.RuntimeFor(ib.NodeID)
}

// nodePushPlan never asks for an imperative push: the node link converges on the database state.
func (s *InboundService) nodePushPlan(*model.Inbound) (runtime.Runtime, bool, bool, error) {
	return nil, false, false, nil
}

func (s *InboundService) AnyNodePending([]int) bool { return false }

func (s *InboundService) NodeIsPending(*int) bool { return false }

func (s *InboundService) restartRemoteNodesOnDisable([]int) {}

// queueNodeResets used to remember client resets for nodes that were offline. Usage now arrives as
// deltas and the counters live only on the panel, so a reset needs no delivery.
func queueNodeResets(*gorm.DB, []string) ([]int, error) { return nil, nil }

func (s *InboundService) deliverNodeResetsNow([]int) {}

func pruneStaleNodeClientIps(int64) error { return nil }

func (s *InboundService) panelGuid() string {
	guid, _ := (&SettingService{}).GetPanelGuid()
	return guid
}

// synthNodeGuid is the stable per-node attribution key.
func synthNodeGuid(nodeID int) string { return fmt.Sprintf("node:%d", nodeID) }

// mergeActivationExpiry: an absolute expiry set on the panel wins over the node's activation time.
func mergeActivationExpiry(existing, node int64) int64 {
	if existing > 0 {
		return existing
	}
	return node
}

var (
	onlineMu       sync.RWMutex
	onlineProvider func() map[int][]string
)

// SetOnlineClientsProvider registers the source of online client emails per node id (the node link).
func SetOnlineClientsProvider(fn func() map[int][]string) {
	onlineMu.Lock()
	onlineProvider = fn
	onlineMu.Unlock()
}

func onlineByNode() map[int][]string {
	onlineMu.RLock()
	fn := onlineProvider
	onlineMu.RUnlock()
	if fn == nil {
		return nil
	}
	return fn()
}

// GetOnlineClients returns the emails of clients that currently have a connection on any node.
func (s *InboundService) GetOnlineClients() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, emails := range onlineByNode() {
		for _, e := range emails {
			if _, dup := seen[e]; !dup {
				seen[e] = struct{}{}
				out = append(out, e)
			}
		}
	}
	return out
}

// GetOnlineClientsByGuid keys online emails by node attribution key.
func (s *InboundService) GetOnlineClientsByGuid() map[string][]string {
	out := map[string][]string{}
	for id, emails := range onlineByNode() {
		out[synthNodeGuid(id)] = emails
	}
	return out
}

// ClientIpInfo is one recorded source IP of a client, with the node it connected through.
type ClientIpInfo struct {
	IP   string `json:"ip"`
	Time string `json:"time"`
	Node string `json:"node"`
}

// GetClientIpsWithNodes is filled by the per-connection accounting of stage 6; until then no IPs are known.
func (s *InboundService) GetClientIpsWithNodes(string) ([]ClientIpInfo, error) {
	return []ClientIpInfo{}, nil
}

func (s *InboundService) GetClientIpsByGuid() (map[string]map[string][]model.ClientIpEntry, error) {
	return map[string]map[string][]model.ClientIpEntry{}, nil
}

func (s *InboundService) GetActiveInboundsByGuid() map[string][]string {
	return map[string][]string{}
}

// GetClientsLastOnline returns each client's last-seen time in unix milliseconds.
func (s *InboundService) GetClientsLastOnline() (map[string]int64, error) {
	var rows []xray.ClientTraffic
	err := database.GetDB().Model(&xray.ClientTraffic{}).Select("email, last_online").Find(&rows).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	result := make(map[string]int64, len(rows))
	for _, r := range rows {
		result[r.Email] = r.LastOnline
	}
	return result, nil
}

// FilterAndSortClientEmails keeps the emails that have a traffic row, ordered by usage descending.
func (s *InboundService) FilterAndSortClientEmails(emails []string) ([]string, []string, error) {
	db := database.GetDB()
	uniq := uniqueNonEmptyStrings(emails)
	clients := make([]xray.ClientTraffic, 0, len(uniq))
	for _, batch := range chunkStrings(uniq, sqliteMaxVars) {
		var page []xray.ClientTraffic
		if err := db.Where("email IN ?", batch).Find(&page).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, err
		}
		clients = append(clients, page...)
	}
	sort.Slice(clients, func(i, j int) bool { return clients[i].Up+clients[i].Down > clients[j].Up+clients[j].Down })
	sorted := make([]string, 0, len(clients))
	found := make(map[string]struct{}, len(clients))
	for _, c := range clients {
		sorted = append(sorted, c.Email)
		found[c.Email] = struct{}{}
	}
	var missing []string
	for _, e := range uniq {
		if _, ok := found[e]; !ok {
			missing = append(missing, e)
		}
	}
	return sorted, missing, nil
}
