// Package nodelink is the panel side of the d256 node link: an internal CA, one-time
// enrollment and the gRPC (mTLS) service the nodes dial into.
package nodelink

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	nodeURIScheme = "d256"
	nodeURIHost   = "node"
	// NodeCertTTL is the lifetime of an issued node certificate.
	NodeCertTTL = 90 * 24 * time.Hour
)

// CA signs node client certificates and the panel's gRPC server certificate.
type CA struct {
	Cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	CertPEM []byte
}

// GenerateCA mints a new CA and returns its PEM-encoded certificate and key.
func GenerateCA() (certPEM, keyPEM []byte, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "d256 node link CA", Organization: []string{"d256"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), nil
}

// LoadCA parses a CA from PEM.
func LoadCA(certPEM, keyPEM []byte) (*CA, error) {
	cb, _ := pem.Decode(certPEM)
	kb, _ := pem.Decode(keyPEM)
	if cb == nil || kb == nil {
		return nil, errors.New("nodelink: CA PEM is invalid")
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, err
	}
	return &CA{Cert: cert, key: key, CertPEM: certPEM}, nil
}

func (ca *CA) Pool() *x509.CertPool {
	p := x509.NewCertPool()
	p.AddCert(ca.Cert)
	return p
}

func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
}

// SerialHex is the canonical string form of a certificate serial stored on the node row.
func SerialHex(c *x509.Certificate) string { return hex.EncodeToString(c.SerialNumber.Bytes()) }

// SignNodeCSR issues a client certificate for nodeID. Only the CSR's public key is used:
// subject and SANs are dictated by the panel, and the key must be ECDSA P-256.
func (ca *CA) SignNodeCSR(csrPEM []byte, nodeID int, ttl time.Duration) (certPEM []byte, serial string, err error) {
	b, _ := pem.Decode(csrPEM)
	if b == nil || b.Type != "CERTIFICATE REQUEST" {
		return nil, "", errors.New("invalid CSR")
	}
	csr, err := x509.ParseCertificateRequest(b.Bytes)
	if err != nil {
		return nil, "", errors.New("invalid CSR")
	}
	if err := csr.CheckSignature(); err != nil {
		return nil, "", errors.New("invalid CSR signature")
	}
	pub, ok := csr.PublicKey.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, "", errors.New("CSR key must be ECDSA P-256")
	}
	sn, err := randomSerial()
	if err != nil {
		return nil, "", err
	}
	uri := &url.URL{Scheme: nodeURIScheme, Host: nodeURIHost, Path: "/" + strconv.Itoa(nodeID)}
	tmpl := &x509.Certificate{
		SerialNumber: sn,
		Subject:      pkix.Name{CommonName: fmt.Sprintf("d256-node-%d", nodeID), Organization: []string{"d256"}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(ttl),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		URIs:         []*url.URL{uri},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, pub, ca.key)
	if err != nil {
		return nil, "", err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, "", err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), SerialHex(cert), nil
}

// IssueServerCert issues the gRPC listener certificate valid for the given hosts (DNS names or IPs).
func (ca *CA) IssueServerCert(hosts []string, ttl time.Duration) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	sn, err := randomSerial()
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: sn,
		Subject:      pkix.Name{CommonName: "d256 panel node link", Organization: []string{"d256"}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(ttl),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else if h != "" {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, &key.PublicKey, ca.key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

// NodeIDFromCert extracts the node id from the d256://node/<id> URI SAN.
func NodeIDFromCert(c *x509.Certificate) (int, error) {
	for _, u := range c.URIs {
		if u.Scheme == nodeURIScheme && u.Host == nodeURIHost {
			id, err := strconv.Atoi(strings.TrimPrefix(u.Path, "/"))
			if err == nil && id > 0 {
				return id, nil
			}
		}
	}
	return 0, errors.New("certificate carries no node identity")
}
