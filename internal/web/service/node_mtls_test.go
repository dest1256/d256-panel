package service

import (
	"testing"

	"github.com/go-playground/validator/v10"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNodeTlsVerifyModeValidatorAcceptsMtls(t *testing.T) {
	v := validator.New(validator.WithRequiredStructEnabled())
	base := model.Node{Name: "n", Address: "node.example.com", Port: 2053, Scheme: "https", ApiToken: "t"}

	for _, m := range []string{"verify", "skip", "pin", "mtls"} {
		n := base
		n.TlsVerifyMode = m
		if err := v.Struct(n); err != nil {
			t.Fatalf("validator rejected valid TlsVerifyMode %q: %v", m, err)
		}
	}
	bad := base
	bad.TlsVerifyMode = "bogus"
	if err := v.Struct(bad); err == nil {
		t.Fatal("validator must reject an unknown TlsVerifyMode")
	}
}
