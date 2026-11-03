package service

import (
	"encoding/json"
	"fmt"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func sameNodeID(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// GetXrayConfigForNode renders the complete Xray JSON a node must run: only the inbounds bound to
// that node, with the panel-local plumbing (gRPC API, metrics endpoint, log files) stripped because
// the node embeds the core and reads its stats in-process.
func (s *XrayService) GetXrayConfigForNode(nodeID int) ([]byte, error) {
	cfg, err := s.buildXrayConfig(&nodeID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return finalizeNodeConfig(raw)
}

func finalizeNodeConfig(raw []byte) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "api")
	delete(m, "metrics")

	level := "warning"
	if lg, ok := m["log"].(map[string]any); ok {
		if l, ok := lg["loglevel"].(string); ok && l != "" {
			level = l
		}
	}
	m["log"] = map[string]any{"access": "none", "loglevel": level}

	if inbounds, ok := m["inbounds"].([]any); ok {
		kept := make([]any, 0, len(inbounds))
		for _, ib := range inbounds {
			if mm, ok := ib.(map[string]any); ok && mm["tag"] == "api" {
				continue
			}
			kept = append(kept, ib)
		}
		m["inbounds"] = kept
	}
	if routing, ok := m["routing"].(map[string]any); ok {
		if rules, ok := routing["rules"].([]any); ok {
			kept := make([]any, 0, len(rules))
			for _, r := range rules {
				if rm, ok := r.(map[string]any); ok && rm["outboundTag"] == "api" {
					continue
				}
				kept = append(kept, r)
			}
			routing["rules"] = kept
		}
	}
	out, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("render node config: %w", err)
	}
	return out, nil
}

// NodeHotOp is one runtime operation a node can apply to a running Xray without restarting it.
type NodeHotOp struct {
	Kind        string // removeUser | addUser | removeInbound | addInbound
	Tag         string
	Email       string
	BestEffort  bool
	UserProto   []byte
	InboundJSON []byte
}

// PlanNodeHotOps turns the difference between two rendered node configs into runtime operations.
// ok is false when the change needs a restart: sections without a reload API, outbound or routing
// changes, protocols that cannot be altered live, or removed clients when the panel is set to
// restart so their live sessions are dropped.
func (s *XrayService) PlanNodeHotOps(oldJSON, newJSON []byte) (ops []NodeHotOp, ok bool) {
	var oldCfg, newCfg xray.Config
	if json.Unmarshal(oldJSON, &oldCfg) != nil || json.Unmarshal(newJSON, &newCfg) != nil {
		return nil, false
	}
	diff, hot := xray.ComputeHotDiff(&oldCfg, &newCfg)
	if !hot || len(diff.TuicRelayChanges) > 0 || len(diff.RemovedOutboundTags) > 0 ||
		len(diff.AddedOutbounds) > 0 || diff.RoutingConfig != nil {
		return nil, false
	}
	if s.restartToDropClients(diff) {
		return nil, false
	}
	for _, u := range diff.RemovedUsers {
		ops = append(ops, NodeHotOp{Kind: "removeUser", Tag: u.Tag, Email: u.Email})
	}
	for _, tag := range diff.RemovedInboundTags {
		ops = append(ops, NodeHotOp{Kind: "removeInbound", Tag: tag})
	}
	for _, ib := range diff.AddedInbounds {
		ops = append(ops, NodeHotOp{Kind: "addInbound", InboundJSON: ib})
	}
	for _, u := range diff.AddedUsers {
		raw, buildable, err := xray.BuildUserProto(u)
		if err != nil || !buildable {
			return nil, false
		}
		// A user that is already present (an edit, or legacy shadowsocks) is replaced.
		ops = append(ops,
			NodeHotOp{Kind: "removeUser", Tag: u.Tag, Email: u.Email, BestEffort: true},
			NodeHotOp{Kind: "addUser", Tag: u.Tag, UserProto: raw})
	}
	return ops, true
}
