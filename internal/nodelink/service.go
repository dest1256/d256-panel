package nodelink

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	settingCACert = "nodeLinkCaCertPem"
	settingCAKey  = "nodeLinkCaKeyPem"
	settingHosts  = "nodeLinkHosts"

	// EnrollTokenTTL is how long a freshly created enrollment token stays redeemable.
	EnrollTokenTTL = time.Hour
)

// ErrEnrollment is returned for every failed redemption; the reason is deliberately not disclosed.
var ErrEnrollment = errors.New("invalid or expired enrollment token")

// Service owns the CA, the enrollment tokens and the gRPC server certificate.
type Service struct {
	// WebDomain, when set, overrides the Host header as the address nodes are told to dial.
	WebDomain func() string

	mu    sync.Mutex
	ca    *CA
	hosts []string
	cert  atomic.Pointer[tls.Certificate]
}

func settingGet(key string) (string, error) {
	var s model.Setting
	err := database.GetDB().Where("key = ?", key).Limit(1).Find(&s).Error
	return s.Value, err
}

func settingSet(key, value string) error {
	db := database.GetDB()
	res := db.Model(&model.Setting{}).Where("key = ?", key).Update("value", value)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return db.Create(&model.Setting{Key: key, Value: value}).Error
	}
	return nil
}

// Init loads (or on first use mints) the CA and issues the server certificate.
func (s *Service) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	certPEM, err := settingGet(settingCACert)
	if err != nil {
		return err
	}
	keyPEM, err := settingGet(settingCAKey)
	if err != nil {
		return err
	}
	switch {
	case certPEM == "" && keyPEM == "":
		c, k, err := GenerateCA()
		if err != nil {
			return err
		}
		if err := settingSet(settingCACert, string(c)); err != nil {
			return err
		}
		if err := settingSet(settingCAKey, string(k)); err != nil {
			return err
		}
		certPEM, keyPEM = string(c), string(k)
	case certPEM == "" || keyPEM == "":
		// Regenerating would silently orphan every enrolled node.
		return errors.New("nodelink: CA is incomplete; refusing to regenerate")
	}
	ca, err := LoadCA([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return err
	}
	s.ca = ca
	h, err := settingGet(settingHosts)
	if err != nil {
		return err
	}
	s.hosts = nil
	for _, x := range strings.Split(h, ",") {
		if x = strings.TrimSpace(x); x != "" {
			s.hosts = append(s.hosts, x)
		}
	}
	return s.reissueLocked()
}

func (s *Service) reissueLocked() error {
	hosts := s.hosts
	if len(hosts) == 0 {
		hosts = []string{"localhost"}
	}
	c, err := s.ca.IssueServerCert(hosts, 365*24*time.Hour)
	if err != nil {
		return err
	}
	s.cert.Store(&c)
	return nil
}

// CA returns the loaded CA (nil before Init).
func (s *Service) CA() *CA {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ca
}

// addHost makes the gRPC certificate valid for host. Only called after a token was redeemed.
func (s *Service) addHost(host string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if host == "" || slices.Contains(s.hosts, host) {
		return nil
	}
	s.hosts = append(s.hosts, host)
	if err := settingSet(settingHosts, strings.Join(s.hosts, ",")); err != nil {
		return err
	}
	return s.reissueLocked()
}

func (s *Service) getServerCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return s.cert.Load(), nil
}

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

// CreateEnrollment issues a fresh one-time token for the node, invalidating its unused older ones.
// The plaintext token is returned once and never stored.
func (s *Service) CreateEnrollment(nodeID int) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	secret := base64.RawURLEncoding.EncodeToString(raw)
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ? AND used_at = 0", nodeID).Delete(&model.NodeEnrollment{}).Error; err != nil {
			return err
		}
		return tx.Create(&model.NodeEnrollment{
			NodeId:      nodeID,
			TokenSha256: hashSecret(secret),
			ExpiresAt:   time.Now().Add(EnrollTokenTTL).Unix(),
		}).Error
	})
	if err != nil {
		return "", err
	}
	return strconv.Itoa(nodeID) + "." + secret, nil
}

// EnrollResult is what a node receives after a successful enrollment.
type EnrollResult struct {
	NodeID  int
	CertPEM []byte
	CAPEM   []byte
}

// Redeem burns the token, signs the CSR and binds the issued certificate to the node.
// host is the address the node used; it is added to the gRPC certificate.
func (s *Service) Redeem(token string, csrPEM []byte, host, nodeVersion string) (*EnrollResult, error) {
	idStr, secret, ok := strings.Cut(token, ".")
	nodeID, err := strconv.Atoi(idStr)
	if !ok || err != nil || nodeID <= 0 || secret == "" || len(token) > 128 {
		return nil, ErrEnrollment
	}
	ca := s.CA()
	if ca == nil {
		return nil, errors.New("nodelink: not initialised")
	}
	// Sign first (side-effect free) so a malformed CSR does not burn the token.
	certPEM, serial, err := ca.SignNodeCSR(csrPEM, nodeID, NodeCertTTL)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&model.NodeEnrollment{}).
			Where("node_id = ? AND token_sha256 = ? AND used_at = 0 AND expires_at > ?", nodeID, hashSecret(secret), now).
			Update("used_at", now)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrEnrollment
		}
		res = tx.Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]any{
			"cert_serial":  serial,
			"enrolled_at":  now,
			"node_version": nodeVersion,
			"status":       "offline",
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrEnrollment
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.addHost(HostOnly(host)); err != nil {
		return nil, err
	}
	return &EnrollResult{NodeID: nodeID, CertPEM: certPEM, CAPEM: ca.CertPEM}, nil
}

// Revoke makes the node's current certificate unusable (the link accepts only the stored serial).
func (s *Service) Revoke(nodeID int) error {
	return database.GetDB().Model(&model.Node{}).Where("id = ?", nodeID).Update("cert_serial", "").Error
}

// HostOnly strips an optional port and IPv6 brackets from a Host header value.
func HostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return strings.Trim(hostport, "[]")
}

var hostRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

// ValidAddress reports whether s is a plausible public hostname or IP for subscription links.
func ValidAddress(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	return hostRe.MatchString(s)
}

// CreateNode adds a pending node and returns it with its first enrollment token.
func (s *Service) CreateNode(name, remark, address string) (*model.Node, string, error) {
	name, address = strings.TrimSpace(name), strings.TrimSpace(address)
	if name == "" || len(name) > 64 {
		return nil, "", errors.New("node name must be 1-64 characters")
	}
	if len(remark) > 256 {
		return nil, "", errors.New("remark is too long")
	}
	if !ValidAddress(address) {
		return nil, "", errors.New("address must be a hostname or IP address")
	}
	n := &model.Node{
		Name: name, Remark: remark, Address: address, Enable: true, Status: "pending",
		Scheme: "https", Port: 2053, BasePath: "/", TlsVerifyMode: "verify", InboundSyncMode: "all",
	}
	if err := database.GetDB().Create(n).Error; err != nil {
		return nil, "", err
	}
	tok, err := s.CreateEnrollment(n.Id)
	if err != nil {
		return nil, "", err
	}
	return n, tok, nil
}

// RenewEnrollment revokes the node's certificate and issues a fresh token (re-enroll flow).
func (s *Service) RenewEnrollment(nodeID int) (string, error) {
	var n model.Node
	if err := database.GetDB().Where("id = ?", nodeID).Limit(1).Find(&n).Error; err != nil {
		return "", err
	}
	if n.Id == 0 {
		return "", errors.New("node not found")
	}
	if err := s.Revoke(nodeID); err != nil {
		return "", err
	}
	database.GetDB().Model(&model.Node{}).Where("id = ?", nodeID).Updates(map[string]any{"status": "pending", "enrolled_at": 0})
	return s.CreateEnrollment(nodeID)
}

// DeleteEnrollments drops all tokens of a deleted node.
func (s *Service) DeleteEnrollments(nodeID int) {
	database.GetDB().Where("node_id = ?", nodeID).Delete(&model.NodeEnrollment{})
}

var (
	defaultMu  sync.RWMutex
	defaultSvc *Service
	defaultSrv *Server
)

// SetDefault registers the running link for the web controllers.
func SetDefault(svc *Service, srv *Server) {
	defaultMu.Lock()
	defaultSvc, defaultSrv = svc, srv
	defaultMu.Unlock()
}

// Default returns the running link service and server (nil before the panel started them).
func Default() (*Service, *Server) {
	defaultMu.RLock()
	defer defaultMu.RUnlock()
	return defaultSvc, defaultSrv
}
