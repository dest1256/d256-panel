package nodelink

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestValidNodeVersion(t *testing.T) {
	for _, ok := range []string{"v0.0.1", "v1.2.3", "v10.20.30", "v1.2.3-rc.1", "v1.2.3-beta"} {
		if !ValidNodeVersion(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "1.2.3", "v1", "v1.2", "v1.2.3/../x", "v1.2.3;rm", "latest", "v01.2.3", " v1.2.3", "v1.2.3\n", strings.Repeat("v1.2.3-a", 10)} {
		if ValidNodeVersion(bad) {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestRequestUpgradeReachesTheNodeAndTheResultIsRecorded(t *testing.T) {
	svc := setup(t)
	n1 := newNode(t, "n1")
	srv := startServer(t, svc)

	if err := srv.Hub().RequestUpgrade(n1, "v1.2.3"); !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("an offline node: %v", err)
	}

	st, cancel := openStream(t, svc, srv, n1, 21)
	defer cancel()
	waitNode(t, n1, func(n model.Node) bool { return n.Status == "online" })
	if err := srv.Hub().RequestUpgrade(n1, "latest; rm -rf /"); err == nil {
		t.Fatal("a malformed version must be refused")
	}
	if err := srv.Hub().RequestUpgrade(n1, "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	m, err := recvUntil(st, func(m *pb.PanelToNode) bool { return m.GetUpgrade() != nil })
	if err != nil {
		t.Fatal(err)
	}
	if m.GetUpgrade().Version != "v1.2.3" {
		t.Fatalf("request = %+v", m.GetUpgrade())
	}
	if err := srv.Hub().RequestUpgrade(n1, "v1.2.4"); err == nil {
		t.Fatal("a second update while one is running must be refused")
	}

	_ = st.Send(&pb.NodeToPanel{Msg: &pb.NodeToPanel_UpgradeResult{UpgradeResult: &pb.UpgradeResult{Version: "v1.2.3", Error: "signature check failed"}}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		var got UpgradeState
		for _, sn := range srv.Hub().Snapshots() {
			if sn.NodeID == n1 {
				got = sn.Upgrade
			}
		}
		if got.State == "failed" {
			if got.Version != "v1.2.3" || got.Error != "signature check failed" {
				t.Fatalf("recorded %+v", got)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("result not recorded: %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	// After a failure another attempt is allowed.
	if err := srv.Hub().RequestUpgrade(n1, "v1.2.3"); err != nil {
		t.Fatalf("retry after a failure: %v", err)
	}
}

func TestLatestNodeReleaseReadsTheTagAndCaches(t *testing.T) {
	var hits int
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/repos/dest1256/d256-node/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.4.0","prerelease":false}`))
	}))
	defer api.Close()
	c := &releaseClient{base: api.URL}

	tag, _, err := c.latest(context.Background(), false)
	if err != nil || tag != "v1.4.0" {
		t.Fatalf("got %q %v", tag, err)
	}
	if _, _, _ = c.latest(context.Background(), false); hits != 1 {
		t.Fatalf("the answer must be cached, hits = %d", hits)
	}
	if _, _, _ = c.latest(context.Background(), true); hits != 2 {
		t.Fatalf("force must refetch, hits = %d", hits)
	}
}

func TestLatestNodeReleaseFailuresAreReadable(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   string
	}{
		"none yet":      {404, `{}`, "no node release"},
		"server error":  {500, `{}`, "answered 500"},
		"garbage":       {200, `not json`, "unreadable"},
		"hostile tag":   {200, `{"tag_name":"v1.0.0/../../x"}`, "not a version"},
		"missing tag":   {200, `{}`, "not a version"},
		"rate limited":  {403, `{}`, "answered 403"},
		"unexpected ok": {200, `{"tag_name":"latest"}`, "not a version"},
	}
	for name, tc := range cases {
		api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		c := &releaseClient{base: api.URL}
		_, _, err := c.latest(context.Background(), true)
		api.Close()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want %q", name, err, tc.want)
		}
	}
	t.Setenv("XUI_NODE_RELEASE_REPO", "../evil")
	if NodeReleaseRepo() != DefaultNodeReleaseRepo {
		t.Error("an invalid repository name must fall back to the default")
	}
	t.Setenv("XUI_NODE_RELEASE_REPO", "someone/fork-of-node")
	if NodeReleaseRepo() != "someone/fork-of-node" {
		t.Error("a valid repository name must be used")
	}
}
