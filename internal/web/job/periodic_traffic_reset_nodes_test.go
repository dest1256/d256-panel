package job

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// resetGate holds every node reset open until released, counting how many nodes
// the job reaches at once.
type resetGate struct {
	entered atomic.Int32
	release chan struct{}
	once    sync.Once
}

func (g *resetGate) open() { g.once.Do(func() { close(g.release) }) }

func (g *resetGate) waitAll(t *testing.T, want int32) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for g.entered.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("periodic reset reached %d of %d hanging nodes, want all of them at once", g.entered.Load(), want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// resetNode is a node hosting inboundTag whose every traffic reset hangs until the
// gate opens; it lists the inbound so the master can resolve its node-side id.
func resetNode(t *testing.T, gate *resetGate, name, inboundTag string) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if strings.HasSuffix(r.URL.Path, "/panel/api/inbounds/list") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"success":true,"obj":[{"id":1,"tag":%q}]}`, inboundTag)
			return
		}
		if strings.Contains(r.URL.Path, "resetTraffic") {
			gate.entered.Add(1)
			select {
			case <-r.Context().Done():
			case <-gate.release:
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(srv.Close)
	host, port, _ := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")
	portNum, _ := strconv.Atoi(port)
	node := &model.Node{
		Name: name, Scheme: "http", Address: host, Port: portNum, BasePath: "/", ApiToken: "tok",
		Enable: true, Status: "online", AllowPrivateAddress: true, TlsVerifyMode: "verify",
	}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatalf("create node: %v", err)
	}
	return node.Id
}

func runResetJobAgainstGate(t *testing.T, gate *resetGate, want int32) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		NewPeriodicTrafficResetJob("daily", time.UTC).Run()
	}()
	t.Cleanup(func() { gate.open(); <-done })
	gate.waitAll(t, want)
}
