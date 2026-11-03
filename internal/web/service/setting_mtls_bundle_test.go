package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/util/crypto"
)

func mustNodeCAPEM(t *testing.T, name string) string {
	t.Helper()
	ca, err := crypto.GenerateNodeCA(name)
	if err != nil {
		t.Fatalf("GenerateNodeCA(%q): %v", name, err)
	}
	return string(ca.CertPEM)
}
