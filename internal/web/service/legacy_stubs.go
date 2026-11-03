package service

import (
	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// MTProto and AmneziaWG are no longer supported (their engines ran inside the panel process, which
// no longer hosts any proxy). These inert shims keep the inbound/client services compiling until
// the protocol branches are removed from them.

type portConflictContext struct{}

func (s *InboundService) loadPortConflictContext(*gorm.DB, *int) (portConflictContext, error) {
	return portConflictContext{}, nil
}

func (s *InboundService) checkForwardedPortsConflict(portConflictContext, string) string { return "" }

func (s *InboundService) checkAmneziaWGForwardedPorts(*gorm.DB, string) error { return nil }

func (s *InboundService) normalizeAmneziaWGSettings(*model.Inbound, *model.Inbound) error { return nil }

func (s *InboundService) applyLocalMtproto(int) {}

func (s *InboundService) applyLocalAmneziaWG(int) {}

func (s *InboundService) resetMtprotoClientQuota(string) {}

func (s *InboundService) resetMtprotoClientQuotas([]string) {}

func (s *InboundService) resetAllMtprotoQuotas() {}

func defaultAmneziaWGSubnetBases(string) (string, string, error) { return "", "", nil }

func defaultAmneziaWGClients(string, []model.Client, []model.Client, []any, map[string]string) error {
	return nil
}

func amneziawgKernelMode(*model.Inbound) bool { return false }

func amneziawgnetSocksSelfConflict(*model.Inbound, int) string { return "" }

func checkAmneziawgnetSocksRelayCollision(*gorm.DB, int) (*portConflictDetail, error) {
	return nil, nil
}

func checkAmneziawgnetSocksReverseConflict(*gorm.DB, int) (*portConflictDetail, error) {
	return nil, nil
}

func checkAmneziawgnetSocksConflict(*gorm.DB, *model.Inbound, int, transportBits) (*portConflictDetail, error) {
	return nil, nil
}

func amneziawgForwardedPortOwner(*gorm.DB, *model.Inbound, int) (*portConflictDetail, error) {
	return nil, nil
}

func injectMtprotoEgress(any, *model.Inbound) {}

func (s *InboundService) normalizeMtprotoXrayPort(*model.Inbound, string) error { return nil }
