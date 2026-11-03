package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

var errOverrideTargetUnknown = errors.New("override target is not an outbound in the running config")

// resolveOverrideTarget maps target to the outbound tag the core routes to: a
// balancer becomes the _bl_ loopback outbound whose rule feeds it.
func resolveOverrideTarget(cfg *xray.Config, target string) (string, error) {
	routing := map[string]any{}
	if len(cfg.RouterConfig) > 0 {
		if err := json.Unmarshal(cfg.RouterConfig, &routing); err != nil {
			return "", err
		}
	}
	if loopback := balancerLoopbackTag(routing, target); loopback != "" && outboundTagExists(cfg.OutboundConfigs, loopback) {
		return loopback, nil
	}
	if outboundTagExists(cfg.OutboundConfigs, target) {
		return target, nil
	}
	return "", fmt.Errorf("%w: %q", errOverrideTargetUnknown, target)
}

func balancerLoopbackTag(routing map[string]any, balancerTag string) string {
	rules, _ := routing["rules"].([]any)
	for _, r := range rules {
		rule, ok := r.(map[string]any)
		if !ok || rule["balancerTag"] != balancerTag {
			continue
		}
		inboundTags, _ := rule["inboundTag"].([]any)
		if len(inboundTags) == 0 {
			continue
		}
		if tag, ok := inboundTags[0].(string); ok && strings.HasPrefix(tag, "_bl_") {
			return tag
		}
	}
	return ""
}
