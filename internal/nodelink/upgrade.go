package nodelink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/logger"
)

// Self-update of nodes: the panel names a release version, the node does the rest (download, signature
// check, install, restart). The panel never sends a URL or a binary.

const (
	// DefaultNodeReleaseRepo is the repository whose releases the panel offers to nodes.
	DefaultNodeReleaseRepo = "dest1256/d256-node"

	releaseCacheTTL = 30 * time.Minute
	maxReleaseBody  = 1 << 20
)

var (
	versionRe = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$`)
	repoRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,38}/[A-Za-z0-9._-]{1,100}$`)
)

// ValidNodeVersion reports whether v is a release tag like v1.2.3.
func ValidNodeVersion(v string) bool { return len(v) <= 40 && versionRe.MatchString(v) }

// UpgradeState is the last update the panel asked a node for.
type UpgradeState struct {
	Version string
	// State is "requested", "ok" (installed, the node is restarting) or "failed".
	State string
	Error string
	At    time.Time
}

// ErrNodeOffline is returned when an update is requested from a node that is not connected.
var ErrNodeOffline = errors.New("the node is not connected")

// RequestUpgrade asks a connected node to update itself to the given release version.
func (h *Hub) RequestUpgrade(id int, version string) error {
	if !ValidNodeVersion(version) {
		return fmt.Errorf("%q is not a release version like v1.2.3", version)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.sessions[id]
	if s == nil {
		return ErrNodeOffline
	}
	if sn := h.snaps[id]; sn != nil && sn.Upgrade.State == "requested" && time.Since(sn.Upgrade.At) < 25*time.Minute {
		return errors.New("an update of this node is already in progress")
	}
	select {
	case s.upgrade <- version:
	default:
		return errors.New("an update of this node is already in progress")
	}
	if sn := h.snaps[id]; sn != nil {
		sn.Upgrade = UpgradeState{Version: version, State: "requested", At: time.Now()}
	}
	return nil
}

func (s *Server) recordUpgradeResult(nodeID int, r *pb.UpgradeResult) {
	state, msg := "failed", truncate(r.Error, 500)
	if r.Ok {
		state, msg = "ok", ""
		logger.Infof("node %d installed %s and is restarting", nodeID, r.Version)
	} else {
		logger.Warningf("node %d could not update to %s: %s", nodeID, r.Version, msg)
	}
	s.hub.update(nodeID, func(sn *Snapshot) {
		sn.Upgrade = UpgradeState{Version: truncate(r.Version, 40), State: state, Error: msg, At: time.Now()}
	})
}

// releaseClient fetches the latest node release. The host is fixed (GitHub's API); only the repository
// name is configurable, and it is validated.
type releaseClient struct {
	mu      sync.Mutex
	base    string
	tag     string
	err     error
	checked time.Time
}

var releases = &releaseClient{base: "https://api.github.com"}

// NodeReleaseRepo returns the repository releases are read from (XUI_NODE_RELEASE_REPO).
func NodeReleaseRepo() string {
	if v := os.Getenv("XUI_NODE_RELEASE_REPO"); repoRe.MatchString(v) {
		return v
	}
	return DefaultNodeReleaseRepo
}

// LatestNodeRelease returns the tag of the newest published node release (cached for half an hour).
func LatestNodeRelease(ctx context.Context, force bool) (tag string, checked time.Time, err error) {
	return releases.latest(ctx, force)
}

func (c *releaseClient) latest(ctx context.Context, force bool) (string, time.Time, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !force && !c.checked.IsZero() && time.Since(c.checked) < releaseCacheTTL {
		return c.tag, c.checked, c.err
	}
	tag, err := c.fetch(ctx)
	c.tag, c.err, c.checked = tag, err, time.Now()
	return c.tag, c.checked, c.err
}

func (c *releaseClient) fetch(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/repos/"+NodeReleaseRepo()+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "d256-panel")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot reach the release server: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", errors.New("no node release has been published yet")
	case resp.StatusCode != http.StatusOK:
		return "", fmt.Errorf("the release server answered %d", resp.StatusCode)
	}
	var body struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
		Draft      bool   `json:"draft"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBody)).Decode(&body); err != nil {
		return "", errors.New("unreadable answer from the release server")
	}
	if !ValidNodeVersion(body.TagName) {
		return "", fmt.Errorf("the latest release tag %q is not a version", truncate(body.TagName, 40))
	}
	return body.TagName, nil
}
