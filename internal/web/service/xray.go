package service

import (
	"encoding/json"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/util/json_util"
	"github.com/mhsanaei/3x-ui/v3/internal/util/maskcompat"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"

	"go.uber.org/atomic"
)

var (
	isNeedXrayRestart atomic.Bool // Indicates that restart was requested for Xray
	xrayState         xrayLifecycle
)

type xrayLifecycle struct {
	mu      sync.RWMutex
	process *xray.Process
	result  string
	// heldBack is why the running core still serves the previous config.
	heldBack string
}

func (s *xrayLifecycle) snapshot() (*xray.Process, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.process, s.result
}

func (s *xrayLifecycle) replace(process *xray.Process) {
	s.mu.Lock()
	s.process = process
	s.result = ""
	s.heldBack = ""
	s.mu.Unlock()
}

func (s *xrayLifecycle) storeResult(process *xray.Process, result string) {
	s.mu.Lock()
	if s.process == process && s.result == "" {
		s.result = result
	}
	s.mu.Unlock()
}

func currentXrayProcess() *xray.Process {
	process, _ := xrayState.snapshot()
	return process
}

// ErrXrayNotRunning reports an operation that requires a running core when xray is stopped.
var ErrXrayNotRunning = xray.ErrXrayNotRunning

// XrayService provides business logic for Xray process management.
// It handles starting, stopping, restarting Xray, and managing its configuration.
type XrayService struct {
	inboundService InboundService
	settingService SettingService
}

// RemoveIndex removes an element at the specified index from a slice.
// Returns a new slice with the element removed.
func RemoveIndex(s []any, index int) []any {
	return append(s[:index], s[index+1:]...)
}

// GetXrayConfig retrieves and builds the Xray configuration from settings and inbounds.
func (s *XrayService) GetXrayConfig() (*xray.Config, error) {
	return s.buildXrayConfig(nil)
}

// buildXrayConfig renders the Xray config for the inbounds of one node. A nil nodeID selects the
// legacy panel-local inbounds (removed together with the local Xray).
func (s *XrayService) buildXrayConfig(nodeID *int) (*xray.Config, error) {
	templateConfig, err := s.settingService.GetXrayConfigTemplate()
	if nodeID != nil {
		templateConfig, _, err = s.settingService.GetXrayConfigTemplateForNode(*nodeID)
	}
	if err != nil {
		return nil, err
	}

	xrayConfig := &xray.Config{}
	err = json.Unmarshal([]byte(templateConfig), xrayConfig)
	if err != nil {
		return nil, err
	}
	xrayConfig.LogConfig = resolveXrayLogPaths(xrayConfig.LogConfig)
	xrayConfig.API = ensureAPIServices(xrayConfig.API)
	xrayConfig.Policy = ensureStatsPolicy(xrayConfig.Policy)
	xrayConfig.RouterConfig = stripDisabledRules(xrayConfig.RouterConfig)
	// A pasted or restored template can carry pre-#6258 XHTTP keys, a pre-26.10.10 xdns mask or a
	// pre-26.9.9 quicParams.udpHop, which the core ignores, or a udphop not last, which it cannot dial.
	xrayConfig.OutboundConfigs = healOutboundStreams(xrayConfig.OutboundConfigs,
		liftXhttpSessionIDKeys, upgradeStreamLegacyXdns, upgradeStreamLegacyUDPHop, normalizeStreamUDPHop)
	xrayConfig.OutboundConfigs = dropXDriveOutboundsRefusedByCore(xrayConfig.OutboundConfigs)
	if xray.KernelTunBlocked() {
		xrayConfig.OutboundConfigs = forceUserspaceWireGuardTun(xrayConfig.OutboundConfigs)
	}

	_, _, _ = s.inboundService.AddTraffic(nil, nil)

	inbounds, err := s.inboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		if !inbound.Enable {
			continue
		}
		if !sameNodeID(inbound.NodeID, nodeID) {
			continue
		}
		if inbound.Protocol == model.MTProto || inbound.Protocol == model.AmneziaWG || inbound.Protocol == model.TUIC {
			continue
		}
		settings := map[string]any{}
		_ = json.Unmarshal([]byte(inbound.Settings), &settings)
		flowAllowed := !inbound.DisableFlow &&
			inboundCanEnableTlsFlow(string(inbound.Protocol), inbound.StreamSettings, inbound.Settings)
		var wireguardClientsByEmail map[string]model.Client
		if inbound.Protocol == model.WireGuard {
			inboundClients, _ := ParseInboundSettingsClients(inbound.Settings)
			if len(inboundClients) > 0 {
				wireguardClientsByEmail = make(map[string]model.Client, len(inboundClients))
				for _, client := range inboundClients {
					wireguardClientsByEmail[strings.ToLower(strings.TrimSpace(client.Email))] = client
				}
			}
		}

		dbClients, listErr := s.inboundService.clientService.ListForInbound(nil, inbound.Id)
		if listErr != nil {
			return nil, listErr
		}

		clientStats := inbound.ClientStats
		enableMap := make(map[string]bool, len(clientStats))
		for _, clientTraffic := range clientStats {
			enableMap[clientTraffic.Email] = clientTraffic.Enable
		}

		finalClients := make([]any, 0, len(dbClients))
		var wgPeers []any
		for i := range dbClients {
			c := dbClients[i]
			if enable, exists := enableMap[c.Email]; exists && !enable {
				logger.Infof("Remove Inbound User %s due to expiration or traffic limit", c.Email)
				continue
			}
			if !c.Enable {
				continue
			}
			flow := c.Flow
			if flow == "xtls-rprx-vision-udp443" {
				flow = "xtls-rprx-vision"
			}
			if !flowAllowed {
				flow = ""
			}
			entry := map[string]any{"email": c.Email}
			switch inbound.Protocol {
			case model.VLESS:
				if c.ID != "" {
					entry["id"] = c.ID
				}
				if flow != "" {
					entry["flow"] = flow
				}
				if c.Reverse != nil {
					entry["reverse"] = c.Reverse
				}
			case model.VMESS:
				if c.ID != "" {
					entry["id"] = c.ID
				}
				if c.Security != "" {
					entry["security"] = c.Security
				}
			case model.Trojan:
				if c.Password != "" {
					entry["password"] = c.Password
				}
				if flow != "" {
					entry["flow"] = flow
				}
			case model.Shadowsocks:
				if c.Password != "" {
					entry["password"] = c.Password
				}
			case model.Hysteria:
				if c.Auth != "" {
					entry["auth"] = c.Auth
				}
			case model.MASQUE:
				if c.Password != "" {
					entry["password"] = c.Password
				}
			case model.WireGuard:
				if inboundClient, ok := wireguardClientsByEmail[strings.ToLower(strings.TrimSpace(c.Email))]; ok {
					c.AllowedIPs = inboundClient.AllowedIPs
					c.PreSharedKey = inboundClient.PreSharedKey
					c.PublicKey = inboundClient.PublicKey
					// #6731: a nil settings keepalive must not keep the other tunnel's value.
					if inboundClient.KeepAlive == nil {
						c.KeepAlive = nil
					} else {
						keepalive := *inboundClient.KeepAlive
						c.KeepAlive = &keepalive
					}
				}
				wgPeers = append(wgPeers, model.WireguardPeerFromClient(c))
				continue
			}
			finalClients = append(finalClients, entry)
		}

		var mutated bool
		if inbound.Protocol == model.WireGuard {
			delete(settings, "clients")
			if wgPeers == nil {
				wgPeers = []any{}
			}
			settings["peers"] = wgPeers
			mutated = true
		} else {
			_, hadClients := settings["clients"]
			mutated = hadClients || len(finalClients) > 0
			if mutated {
				settings["clients"] = finalClients
			}
		}

		if inboundCanHostFallbacks(inbound) {
			fallbacks, fbErr := s.inboundService.fallbackService.BuildFallbacksJSON(nil, inbound.Id)
			if fbErr != nil {
				return nil, fbErr
			}
			if len(fallbacks) > 0 {
				generic := make([]any, 0, len(fallbacks))
				for _, f := range fallbacks {
					generic = append(generic, f)
				}
				settings["fallbacks"] = generic
				mutated = true
			}
		}

		if mutated {
			modifiedSettings, err := json.MarshalIndent(settings, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.Settings = string(modifiedSettings)
		}

		if len(inbound.StreamSettings) > 0 {
			// Unmarshal stream JSON
			var stream map[string]any
			_ = json.Unmarshal([]byte(inbound.StreamSettings), &stream)

			// Remove the "settings" field under "tlsSettings" and "realitySettings"
			tlsSettings, ok1 := stream["tlsSettings"].(map[string]any)
			realitySettings, ok2 := stream["realitySettings"].(map[string]any)
			if ok1 || ok2 {
				if ok1 {
					delete(tlsSettings, "settings")
				} else if ok2 {
					delete(realitySettings, "settings")
				}
			}

			delete(stream, "externalProxy")

			// finalmask.tcp + REALITY panics Xray-core on the first connection
			// (XTLS/Xray-core#6453). AddInbound/UpdateInbound reject this
			// combination at save time, but a row saved before that guard
			// existed (upgrade, node sync, restored backup, direct DB edit)
			// would still crash Xray on the next restart without this — drop
			// it here too, the same way liftXhttpSessionIDKeys and
			// HealShadowsocksClientMethods heal other legacy data in place.
			if len(finalMaskRealityTcpMasks(stream)) > 0 {
				logger.Warningf("Inbound %q: dropping finalmask, incompatible with REALITY security (crashes Xray-core, see XTLS/Xray-core#6453)", inbound.Tag)
				delete(stream, "finalmask")
			}

			dropEmptyRandPackets(stream["finalmask"])

			if dropped := stripIncompleteXmcMasks(stream); dropped > 0 {
				logger.Warningf("Inbound %q: dropping %d XMC finalmask mask(s) without complete Minecraft profiles — reconfigure them to restore the obfuscation (see XTLS/Xray-core#6487)", inbound.Tag, dropped)
			}

			// A row that skipped the save path can still carry an xdns shape older than 26.10.10.
			maskcompat.UpgradeLegacyXdns(stream["finalmask"])

			// xray-core v26.6.22 (#6258) renamed the XHTTP session keys and
			// kept no fallback. Lift legacy sessionPlacement/sessionKey onto the
			// new names here so inbounds stored before the rename keep working
			// without the admin re-saving them.
			liftXhttpSessionIDKeys(stream)

			newStream, err := json.MarshalIndent(stream, "", "  ")
			if err != nil {
				return nil, err
			}
			inbound.StreamSettings = string(newStream)
		}

		if inbound.Protocol == model.Shadowsocks {
			if healed, ok := model.HealShadowsocksClientMethods(inbound.Settings); ok {
				inbound.Settings = healed
			}
		}

		inboundConfig := inbound.GenXrayInboundConfig()
		xrayConfig.InboundConfigs = append(xrayConfig.InboundConfigs, *inboundConfig)
	}
	xrayConfig.InboundConfigs = dropInboundsRefusedByXDrive(xrayConfig.InboundConfigs)

	// Merge subscription-derived outbounds (if any) into the final outbounds array.
	// These are additive: each subscription is placed before or after the template
	// outbounds based on its Prepend flag, ordered by Priority. Tags assigned by the
	// subscription service are kept stable across refreshes so that balancers and
	// routing rules continue to work.
	subSvc := &OutboundSubscriptionService{}
	if prepend, appendList, err := subSvc.activeOutboundsSplit(); err == nil && (len(prepend) > 0 || len(appendList) > 0) {
		mergeSubscriptionOutbounds(xrayConfig, prepend, appendList)
	}

	xrayConfig.RouterConfig = dropRefusedScript(xrayConfig.RouterConfig, "routing", routingScriptRefused)
	xrayConfig.DNSConfig = dropRefusedScript(xrayConfig.DNSConfig, "dns", dnsScriptRefused)
	return xrayConfig, nil
}

func outboundTagExists(outbounds json_util.RawMessage, tag string) bool {
	var parsed []struct {
		Tag string `json:"tag"`
	}
	if tag == "" || json.Unmarshal(outbounds, &parsed) != nil {
		return false
	}
	for _, outbound := range parsed {
		if outbound.Tag == tag {
			return true
		}
	}
	return false
}

// mergeSubscriptionOutbounds appends the subscription outbounds to the
// OutboundConfigs array of the xray config. It works on the already-unmarshaled
// template so that manually configured outbounds are never overwritten.
//
// Safety: if we cannot parse the template's outbounds array, we leave
// OutboundConfigs exactly as it came from the template (we do not inject
// subscription outbounds). This prevents us from accidentally dropping the
// user's manually configured outbounds when the template is in a weird state.
func mergeSubscriptionOutbounds(cfg *xray.Config, prepend, appendList []any) {
	if len(prepend) == 0 && len(appendList) == 0 {
		return
	}
	var templateOutbounds []any
	if len(cfg.OutboundConfigs) > 0 {
		if err := json.Unmarshal(cfg.OutboundConfigs, &templateOutbounds); err != nil {
			// Corrupt template outbounds — do not touch the field at all.
			// The user will see problems on Xray start / next save.
			return
		}
	}
	var merged []any
	merged = append(merged, prepend...)
	merged = append(merged, templateOutbounds...)
	merged = append(merged, appendList...)
	combined, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return
	}
	cfg.OutboundConfigs = json_util.RawMessage(combined)
}

// ensureAPIServices guarantees the gRPC services the panel depends on are
// listed in the generated config's api block: HandlerService and StatsService
// have always been required for inbound/user management and traffic polling,
// and RoutingService enables hot routing reload on templates saved before it
// was added to the default template. The stored template itself is not
// modified — only the generated runtime config.
func ensureAPIServices(api json_util.RawMessage) json_util.RawMessage {
	if len(api) == 0 {
		// No api block means the panel's API integration is deliberately
		// disabled; don't resurrect it behind the user's back.
		return api
	}
	var parsed map[string]any
	if err := json.Unmarshal(api, &parsed); err != nil {
		return api
	}
	services, _ := parsed["services"].([]any)
	have := make(map[string]bool, len(services))
	for _, svc := range services {
		if name, ok := svc.(string); ok {
			have[name] = true
		}
	}
	added := false
	for _, name := range []string{"HandlerService", "StatsService", "RoutingService"} {
		if !have[name] {
			services = append(services, name)
			added = true
		}
	}
	if !added {
		return api
	}
	parsed["services"] = services
	out, err := json.Marshal(parsed)
	if err != nil {
		return api
	}
	return out
}

// ensureStatsPolicy guarantees every policy level in the generated config has
// statsUserOnline enabled, so the core tracks per-email online IPs for the
// panel's online view and access-log-free IP limiting. Generated clients carry
// no explicit level, so level "0" is created when absent. The flag is panel
// infrastructure and is forced on even over an explicit false in the template,
// same as the api services above. An entirely missing or unparsable policy
// block is left alone; the stored template itself is never modified — only the
// generated runtime config.
func ensureStatsPolicy(policy json_util.RawMessage) json_util.RawMessage {
	if len(policy) == 0 {
		return policy
	}
	var parsed map[string]any
	if err := json.Unmarshal(policy, &parsed); err != nil {
		return policy
	}
	levels, _ := parsed["levels"].(map[string]any)
	if levels == nil {
		levels = make(map[string]any)
	}
	if _, ok := levels["0"]; !ok {
		levels["0"] = map[string]any{}
	}
	changed := false
	for _, raw := range levels {
		level, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if enabled, ok := level["statsUserOnline"].(bool); !ok || !enabled {
			level["statsUserOnline"] = true
			changed = true
		}
	}
	if !changed {
		return policy
	}
	parsed["levels"] = levels
	out, err := json.Marshal(parsed)
	if err != nil {
		return policy
	}
	return out
}

// caseVariantKeys returns every key of parsed that equals want ignoring case,
// lowest first so the fold is deterministic when several variants are present.
func caseVariantKeys(parsed map[string]any, want string) []string {
	var keys []string
	for key := range parsed {
		if strings.EqualFold(key, want) {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

func resolveXrayLogPaths(logCfg json_util.RawMessage) json_util.RawMessage {
	if len(logCfg) == 0 {
		return logCfg
	}
	var parsed map[string]any
	if err := json.Unmarshal(logCfg, &parsed); err != nil {
		return logCfg
	}
	changed := false
	for _, key := range []string{"access", "error"} {
		// xray-core decodes this object with encoding/json, whose case-insensitive
		// field match makes "Access" reach AccessLog too — fold every variant.
		variants := caseVariantKeys(parsed, key)
		value, hasValue := parsed[key]
		for _, variant := range variants {
			if variant == key {
				continue
			}
			if !hasValue {
				value, hasValue = parsed[variant], true
			}
			delete(parsed, variant)
			changed = true
		}
		v, ok := value.(string)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(v)
		if trimmed == "" || strings.EqualFold(trimmed, "none") {
			if changed {
				parsed[key] = v
			}
			continue
		}
		base := path.Base(filepath.ToSlash(trimmed))
		if base == "" || base == "." || base == ".." || base == "/" {
			continue
		}
		confined := filepath.Join(config.GetLogFolder(), base)
		if confined == trimmed {
			continue
		}
		parsed[key] = confined
		changed = true
	}
	if !changed {
		return logCfg
	}
	out, err := json.Marshal(parsed)
	if err != nil {
		return logCfg
	}
	return out
}

// stripDisabledRules removes routing rules marked `enabled: false` from the
// generated runtime config and strips panel-only keys (`enabled`, `comment`)
// from the rest, since xray-core has no such fields. The internal api rule is
// always kept (see isApiRule) so traffic stats can't be toggled off. The stored
// template is untouched — only the generated config is filtered.
func stripDisabledRules(routerCfg json_util.RawMessage) json_util.RawMessage {
	if len(routerCfg) == 0 {
		return routerCfg
	}
	var parsed map[string]any
	if err := json.Unmarshal(routerCfg, &parsed); err != nil {
		return routerCfg
	}
	rules, ok := parsed["rules"].([]any)
	if !ok || len(rules) == 0 {
		return routerCfg
	}

	var activeRules []any
	changed := false
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]any)
		if !ok {
			activeRules = append(activeRules, rawRule)
			continue
		}

		if enabledRaw, exists := rule["enabled"]; exists {
			// The internal api rule carries traffic stats and must never be
			// dropped, even if it was somehow marked disabled.
			enabled, ok := enabledRaw.(bool)
			if ok && !enabled && !isApiRule(rule) {
				changed = true
				continue
			}
			delete(rule, "enabled")
			changed = true
		}
		if _, exists := rule["comment"]; exists {
			delete(rule, "comment")
			changed = true
		}
		activeRules = append(activeRules, rule)
	}

	if !changed {
		return routerCfg
	}

	parsed["rules"] = activeRules
	out, err := json.Marshal(parsed)
	if err != nil {
		return routerCfg
	}
	return out
}

// BalancerStatus is the live view of one balancer for the panel UI. Running
// is false when the balancer isn't present in the running core (e.g. xray is
// stopped or the balancer hasn't been saved/applied yet).
type BalancerStatus struct {
	Tag      string   `json:"tag"`
	Running  bool     `json:"running"`
	Override string   `json:"override"`
	Selected []string `json:"selected"`
}

// restartToDropClients reports whether a diff that strands clients must be
// applied by restarting instead of through the API.
func (s *XrayService) restartToDropClients(diff *xray.HotDiff) bool {
	if diff == nil || !diff.DropsUsers() {
		return false
	}
	restart, err := s.settingService.GetRestartXrayOnClientDisable()
	if err != nil {
		logger.Warning("get RestartXrayOnClientDisable failed:", err)
		return false
	}
	return restart
}

// SetToNeedRestart marks that Xray needs to be restarted.
func (s *XrayService) SetToNeedRestart() {
	isNeedXrayRestart.Store(true)
}

// liftXhttpSessionIDKeys renames the legacy XHTTP session keys
// (sessionPlacement/sessionKey) to the v26.6.22 #6258 names
// (sessionIDPlacement/sessionIDKey) inside a streamSettings map. xray-core kept
// no fallback for the old names, so a config stored before the rename would be
// silently ignored by the engine. Returns true if it changed anything.
func liftXhttpSessionIDKeys(stream map[string]any) bool {
	xhttp, ok := stream["xhttpSettings"].(map[string]any)
	if !ok {
		return false
	}
	changed := false
	for legacy, renamed := range map[string]string{
		"sessionPlacement": "sessionIDPlacement",
		"sessionKey":       "sessionIDKey",
	} {
		v, has := xhttp[legacy]
		if !has {
			continue
		}
		if _, exists := xhttp[renamed]; !exists {
			xhttp[renamed] = v
		}
		delete(xhttp, legacy)
		changed = true
	}
	return changed
}

// healOutboundStreams applies every heal to each outbound's streamSettings, returning the
// original bytes when none changed anything so the hot-reload diff sees no edit.
func healOutboundStreams(raw json_util.RawMessage, heals ...func(stream map[string]any) bool) json_util.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var outbounds []map[string]any
	if err := json.Unmarshal(raw, &outbounds); err != nil {
		return raw
	}
	changed := false
	for _, ob := range outbounds {
		if stream, ok := ob["streamSettings"].(map[string]any); ok {
			for _, heal := range heals {
				if heal(stream) {
					changed = true
				}
			}
		}
	}
	if !changed {
		return raw
	}
	if rewritten, err := json.Marshal(outbounds); err == nil {
		return rewritten
	}
	return raw
}

// dropInboundsRefusedByXDrive keeps out an XDRIVE inbound stored before xray-core 26.10.17 made it
// VLESS-encryption only (#7182); the core would refuse the whole config and stop every inbound.
func dropInboundsRefusedByXDrive(inbounds []xray.InboundConfig) []xray.InboundConfig {
	return slices.DeleteFunc(inbounds, func(ib xray.InboundConfig) bool {
		err := validateXDriveInbound(ib.Protocol, string(ib.Settings), string(ib.StreamSettings))
		if err != nil {
			logger.Warningf("Inbound %q: not started, %v", ib.Tag, err)
		}
		return err != nil
	})
}

// dropXDriveOutboundsRefusedByCore keeps out an XDRIVE outbound stored before xray-core 26.10.17
// required VLESS encryption and mux on it (#7182), which would stop the whole core at startup.
func dropXDriveOutboundsRefusedByCore(raw json_util.RawMessage) json_util.RawMessage {
	var outbounds []json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &outbounds) != nil {
		return raw
	}
	kept := make([]json.RawMessage, 0, len(outbounds))
	for _, ob := range outbounds {
		var probe struct {
			Tag            string `json:"tag"`
			StreamSettings struct {
				Network string `json:"network"`
			} `json:"streamSettings"`
		}
		if json.Unmarshal(ob, &probe) == nil && probe.StreamSettings.Network == "xdrive" {
			if err := xray.ValidateOutboundConfig(ob); err != nil {
				logger.Warningf("Outbound %q: not started, xray-core refuses it: %v", probe.Tag, err)
				continue
			}
		}
		kept = append(kept, ob)
	}
	if len(kept) == len(outbounds) {
		return raw
	}
	if rewritten, err := json.Marshal(kept); err == nil {
		return rewritten
	}
	return raw
}

// routingScriptRefused mirrors xray-core 26.10.17's RouterConfig.Build (#7136): a routing
// "script" replaces rules and domainStrategy, so the core refuses a config holding both.
func routingScriptRefused(routing map[string]any) bool {
	script, _ := routing["script"].(string)
	rules, _ := routing["rules"].([]any)
	return script != "" && (routing["domainStrategy"] != nil || len(rules) > 0)
}

// dnsScriptRefused mirrors xray-core 26.10.17's DNSConfig.Build (#7136): a dns "script" refuses the
// fallback and parallel switches, and any server scoped by domains, skipFallback or finalQuery.
func dnsScriptRefused(dns map[string]any) bool {
	if script, _ := dns["script"].(string); script == "" {
		return false
	}
	for _, key := range []string{"disableFallback", "disableFallbackIfMatch", "enableParallelQuery"} {
		if on, _ := dns[key].(bool); on {
			return true
		}
	}
	servers, _ := dns["servers"].([]any)
	return slices.ContainsFunc(servers, func(entry any) bool {
		server, _ := entry.(map[string]any)
		skipFallback, _ := server["skipFallback"].(bool)
		finalQuery, _ := server["finalQuery"].(bool)
		return hasDNSDomains(server["domains"]) || skipFallback || finalQuery
	})
}

func hasDNSDomains(domains any) bool {
	switch v := domains.(type) {
	case []any:
		return len(v) > 0
	case string:
		return v != ""
	}
	return false
}

// dropRefusedScript drops a section's Lua "script" the core would refuse, logging why: the
// panel's own rules and DNS settings stay, since its stats path depends on the api rule.
func dropRefusedScript(raw json_util.RawMessage, section string, refused func(map[string]any) bool) json_util.RawMessage {
	var parsed map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &parsed) != nil || !refused(parsed) {
		return raw
	}
	logger.Warningf("%s: ignoring \"script\" %v, xray-core refuses it beside this %s section's other settings", section, parsed["script"], section)
	delete(parsed, "script")
	if rewritten, err := json.Marshal(parsed); err == nil {
		return rewritten
	}
	return raw
}

func upgradeStreamLegacyXdns(stream map[string]any) bool {
	return maskcompat.UpgradeLegacyXdns(stream["finalmask"])
}

func upgradeStreamLegacyUDPHop(stream map[string]any) bool {
	return maskcompat.UpgradeLegacyUDPHop(stream["finalmask"])
}

func normalizeStreamUDPHop(stream map[string]any) bool {
	return maskcompat.NormalizeUDPHop(stream["finalmask"])
}

// forceUserspaceWireGuardTun moves every WireGuard outbound to the gVisor TUN (#6796). Values
// stay raw so the template's other keys reach the core byte for byte.
func forceUserspaceWireGuardTun(raw json_util.RawMessage) json_util.RawMessage {
	var outbounds []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &outbounds); err != nil {
		return raw
	}
	for _, ob := range outbounds {
		var protocol, tag string
		if json.Unmarshal(ob["protocol"], &protocol) != nil || !strings.EqualFold(protocol, "wireguard") {
			continue
		}
		var settings map[string]json.RawMessage
		if json.Unmarshal(ob["settings"], &settings) != nil || settings == nil {
			continue
		}
		wasUserspace := string(settings["noKernelTun"]) == "true"
		settings["noKernelTun"] = json.RawMessage("true")
		rewritten, err := json.Marshal(settings)
		if err != nil {
			continue
		}
		ob["settings"] = rewritten
		if !wasUserspace {
			_ = json.Unmarshal(ob["tag"], &tag)
			logger.Warningf("Outbound %q: kernel TUN cannot be set up here, running WireGuard on the userspace TUN instead (#6796)", tag)
		}
	}
	rewritten, err := json.Marshal(outbounds)
	if err != nil {
		return raw
	}
	return rewritten
}
