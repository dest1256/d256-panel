package job

import (
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/nodelink"
)

// ConnRetentionJob keeps the per-destination and chart tables within their retention windows.
type ConnRetentionJob struct{}

func NewConnRetentionJob() *ConnRetentionJob { return &ConnRetentionJob{} }

func (j *ConnRetentionJob) Run() {
	if err := nodelink.RunRetention(time.Now()); err != nil {
		logger.Warning("connection statistics retention failed:", err)
	}
}
