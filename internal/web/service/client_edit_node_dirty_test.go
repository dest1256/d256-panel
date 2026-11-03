package service

import (
	"context"
	"sync/atomic"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// dirtyProbeRuntime reads a node's config_dirty flag at the moment an inbound
// pushes, i.e. while the edit's other inbounds are still unapplied.
type dirtyProbeRuntime struct {
	fakeNodeRuntime
	watch  int
	sawSet atomic.Bool
	probed atomic.Bool
}

func (d *dirtyProbeRuntime) UpdateUser(ctx context.Context, ib *model.Inbound, oldEmail string, c model.Client) error {
	var node model.Node
	if err := database.GetDB().Where("id = ?", d.watch).First(&node).Error; err == nil {
		d.sawSet.Store(node.ConfigDirty)
		d.probed.Store(true)
	}
	return d.fakeNodeRuntime.UpdateUser(ctx, ib, oldEmail, c)
}
