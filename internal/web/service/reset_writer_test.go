package service

import (
	"context"
)

type blockingResetRuntime struct {
	fakeNodeRuntime
	reached chan struct{}
	release chan struct{}
}

func (b *blockingResetRuntime) ResetAllTraffics(context.Context) error {
	close(b.reached)
	<-b.release
	return nil
}
