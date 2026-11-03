package service

import (
	"strings"
	"testing"
)

func TestValidateInboundTLSCertificates(t *testing.T) {
	tests := []struct {
		name           string
		streamSettings string
		wantErr        bool
	}{
		{"empty stream", "", false},
		{"whitespace stream", " \t\n", false},
		{"none ignores stale TLS settings", `{"security":"none","tlsSettings":{"certificates":[{}]}}`, false},
		{"reality needs no TLS certificate", `{"security":"reality","realitySettings":{}}`, false},
		{"missing TLS settings", `{"security":"tls"}`, true},
		{"uppercase TLS security", `{"security":"TLS","tlsSettings":{}}`, true},
		{"mixed-case TLS security", `{"security":"Tls","tlsSettings":{}}`, true},
		{"null TLS settings", `{"security":"tls","tlsSettings":null}`, true},
		{"missing certificates", `{"security":"tls","tlsSettings":{}}`, true},
		{"null certificates", `{"security":"tls","tlsSettings":{"certificates":null}}`, true},
		{"empty certificates", `{"security":"tls","tlsSettings":{"certificates":[]}}`, true},
		{"null certificate row", `{"security":"tls","tlsSettings":{"certificates":[null]}}`, true},
		{"empty default file fields", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"","keyFile":""}]}}`, true},
		{"empty default inline fields", `{"security":"tls","tlsSettings":{"certificates":[{"certificate":[],"key":[]}]}}`, true},
		{"whitespace file fields", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":" \t","keyFile":" \n"}]}}`, true},
		{"whitespace inline certificate", `{"security":"tls","tlsSettings":{"certificates":[{"certificate":[" ","\t"],"key":["private key"]}]}}`, true},
		{"whitespace inline key", `{"security":"tls","tlsSettings":{"certificates":[{"certificate":["certificate"],"key":[" ","\n"]}]}}`, true},
		{"missing private key", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/node/cert.pem"}]}}`, true},
		{"missing certificate", `{"security":"tls","tlsSettings":{"certificates":[{"keyFile":"/node/key.pem"}]}}`, true},
		{"verify only", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"verify","certificateFile":"/node/ca.pem"}]}}`, true},
		{"verify with private key still needs server certificate", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"verify","certificateFile":"/node/ca.pem","keyFile":"/node/key.pem"}]}}`, true},
		{"issue needs private key", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"issue","certificateFile":"/node/ca.pem"}]}}`, true},
		{"file credentials with default usage", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"}]}}`, false},
		{"inline credentials", `{"security":"tls","tlsSettings":{"certificates":[{"certificate":["certificate"],"key":["private key"]}]}}`, false},
		{"certificate file and inline key", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/node/cert.pem","key":["private key"]}]}}`, false},
		{"inline certificate and key file", `{"security":"tls","tlsSettings":{"certificates":[{"certificate":["certificate"],"keyFile":"/node/key.pem"}]}}`, false},
		{"encipherment usage", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"encipherment","certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"}]}}`, false},
		{"issue usage", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"issue","certificateFile":"/node/ca.pem","keyFile":"/node/ca-key.pem"}]}}`, false},
		{"unknown usage defaults to encipherment like Xray", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"custom","certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"}]}}`, false},
		{"verify and server certificates", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"verify","certificateFile":"/node/ca.pem"},{"certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"}]}}`, false},
		{"verify usage is case insensitive", `{"security":"tls","tlsSettings":{"certificates":[{"usage":"VERIFY","certificateFile":"/node/ca.pem"},{"certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"}]}}`, false},
		{"empty extra certificate row", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"},{}]}}`, true},
		{"empty extra verify certificate", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"},{"usage":"verify"}]}}`, true},
		{"whitespace certificate file overrides inline content", `{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":" ","certificate":["certificate"],"key":["private key"]}]}}`, true},
		{"whitespace key file overrides inline content", `{"security":"tls","tlsSettings":{"certificates":[{"certificate":["certificate"],"keyFile":" ","key":["private key"]}]}}`, true},
		{"malformed stream", `{"security":"tls"`, true},
		{"malformed TLS settings", `{"security":"tls","tlsSettings":"invalid"}`, true},
		{"malformed certificate list", `{"security":"tls","tlsSettings":{"certificates":{}}}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInboundTLSCertificates(tt.streamSettings)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateInboundTLSCertificates() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateInboundTLSCertificatesIdentifiesIncompleteRow(t *testing.T) {
	err := validateInboundTLSCertificates(`{"security":"tls","tlsSettings":{"certificates":[{"certificateFile":"/node/cert.pem","keyFile":"/node/key.pem"},{"certificateFile":"/node/other.pem"}]}}`)
	if err == nil || !strings.Contains(err.Error(), "TLS certificate 2") || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("expected actionable error for the second certificate's private key, got %v", err)
	}
}
