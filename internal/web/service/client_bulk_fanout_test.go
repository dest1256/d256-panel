package service

import (
	"fmt"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// The bulk ops walked their inbounds one node round-trip at a time, so a client
// spanning several nodes cost the SUM of every node's latency. Each test below
// times out on the barrier unless the pushes overlap.

// TestBulkCreateSerializesTunnelAddressAllocation pins that a bulk create over
// WireGuard inbounds does not overlap: allocation reads every inbound's used-set
// before writing, so two concurrent picks collide and the second is refused.
func TestBulkCreateSerializesTunnelAddressAllocation(t *testing.T) {
	setupBulkDB(t)
	startSerializedWriter(t)
	db := database.GetDB()

	ids := make([]int, 0, 2)
	for i := range 2 {
		ib := &model.Inbound{
			UserId: 1, Enable: true, Port: 51820 + i,
			Tag: fmt.Sprintf("wg-%d", i), Protocol: model.WireGuard,
			Settings: `{"clients":[],"mtu":1420,"secretKey":"QO3O1V0m0Sm1yQ0hVvJ0kM0kQe0mYq0Wc0Zk0Xs0Zm8=","peers":[]}`,
		}
		if err := db.Create(ib).Error; err != nil {
			t.Fatalf("create wg inbound: %v", err)
		}
		ids = append(ids, ib.Id)
	}

	res, _, err := (&ClientService{}).BulkCreate(&InboundService{}, []ClientCreatePayload{
		{Client: model.Client{Email: "a@wg", SubID: "sa", Enable: true}, InboundIds: []int{ids[0]}},
		{Client: model.Client{Email: "b@wg", SubID: "sb", Enable: true}, InboundIds: []int{ids[1]}},
	})
	if err != nil {
		t.Fatalf("BulkCreate over two wg inbounds: %v", err)
	}
	if res.Created != 2 {
		t.Fatalf("created = %d, want 2 — concurrent allocation handed out one address twice: %+v", res.Created, res.Skipped)
	}
}
