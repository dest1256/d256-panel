package nodelink

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

const maxEnrollBody = 64 << 10

// attemptLimiter is a fixed-window per-key attempt counter.
type attemptLimiter struct {
	mu     sync.Mutex
	window time.Time
	counts map[string]int
	limit  int
}

func newLimiter(limit int) *attemptLimiter {
	return &attemptLimiter{counts: map[string]int{}, limit: limit}
}

func (l *attemptLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now := time.Now(); now.Sub(l.window) > time.Minute {
		l.window, l.counts = now, map[string]int{}
	}
	l.counts[key]++
	return l.counts[key] <= l.limit
}

type enrollRequest struct {
	Token       string `json:"token"`
	CSR         string `json:"csr"`
	NodeVersion string `json:"nodeVersion"`
}

type enrollResponse struct {
	NodeID   int    `json:"nodeId"`
	Cert     string `json:"cert"`
	CA       string `json:"ca"`
	GRPCAddr string `json:"grpcAddr"`
}

// HandleEnroll serves POST .../d256/enroll. It sits outside the session/CSRF middleware:
// the node authenticates with its one-time token. Attempts are limited per IP and globally.
func (s *Service) HandleEnroll(grpcPort int) gin.HandlerFunc {
	perIP, global := newLimiter(10), newLimiter(120)
	return func(c *gin.Context) {
		if !perIP.allow(c.ClientIP()) || !global.allow("*") {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "too many attempts"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEnrollBody)
		var req enrollRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "bad request"})
			return
		}
		host := c.Request.Host
		if s.WebDomain != nil {
			if d := s.WebDomain(); d != "" {
				host = d
			}
		}
		res, err := s.Redeem(req.Token, []byte(req.CSR), host, truncate(req.NodeVersion, 64))
		if err != nil {
			if errors.Is(err, ErrEnrollment) {
				c.JSON(http.StatusForbidden, gin.H{"error": ErrEnrollment.Error()})
				return
			}
			logger.Warning("node enrollment failed:", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "enrollment failed"})
			return
		}
		c.JSON(http.StatusOK, enrollResponse{
			NodeID:   res.NodeID,
			Cert:     string(res.CertPEM),
			CA:       string(res.CAPEM),
			GRPCAddr: net.JoinHostPort(HostOnly(host), strconv.Itoa(grpcPort)),
		})
		logger.Infof("node %d enrolled", res.NodeID)
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
