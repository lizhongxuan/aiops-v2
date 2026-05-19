package builtin

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"runner/modules"
	"runner/workflow"
)

func TestTCPPing(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	res, err := NewTCPPing().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{"host": host, "port": port}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Output["ok"] != true {
		t.Fatalf("ok = %#v", res.Output["ok"])
	}
	if res.Output["reachable"] != true || res.Output["latency_ms"] == nil || res.Output["remote_addr"] == "" {
		t.Fatalf("tcp output missing probe contract fields: %#v", res.Output)
	}
	if res.Output["schema_version"] != modules.RunnerResultSchemaVersion {
		t.Fatalf("schema_version = %#v", res.Output["schema_version"])
	}
	if data, ok := res.Output["data"].(map[string]any); !ok || data["reachable"] != true {
		t.Fatalf("envelope data = %#v", res.Output["data"])
	}

	_ = ln.Close()
	res, err = NewTCPPing().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{"host": host, "port": port, "timeout": "50ms"}},
	})
	if err == nil {
		t.Fatal("expected closed listener to be unreachable")
	}
	if res.Output["ok"] != false {
		t.Fatalf("closed listener ok = %#v", res.Output["ok"])
	}
	if res.Output["reachable"] != false {
		t.Fatalf("closed listener reachable = %#v", res.Output["reachable"])
	}
}

func TestTCPPingValidation(t *testing.T) {
	_, err := NewTCPPing().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{"port": 80}},
	})
	if err == nil || !strings.Contains(err.Error(), "host is required") {
		t.Fatalf("expected missing host error, got %v", err)
	}

	_, err = NewTCPPing().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{"host": "127.0.0.1", "port": "not-a-port"}},
	})
	if err == nil {
		t.Fatalf("expected invalid port error")
	}
}

func TestHTTPCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"status":{"ready":true},"message":"healthy"}`))
	}))
	defer server.Close()

	res, err := NewHTTPCheck().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"url":             server.URL,
			"expected_status": []any{200, 201},
			"body_contains":   "healthy",
			"json_path":       "$.status.ready",
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Output["status_code"] != 201 {
		t.Fatalf("status_code = %#v", res.Output["status_code"])
	}
	if res.Output["matched"] != true || res.Output["body_contains_matched"] != true || res.Output["json_path_value"] != true {
		t.Fatalf("http check match fields = %#v", res.Output)
	}
	if res.Output["body_excerpt"] == "" || res.Output["latency_ms"] == nil {
		t.Fatalf("http check missing output fields = %#v", res.Output)
	}
}

func TestSSLExpiryCheck(t *testing.T) {
	cert, err := selfSignedCert()
	if err != nil {
		t.Fatalf("cert: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_, _ = conn.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
			_ = conn.Close()
		}
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	res, err := NewSSLExpiryCheck().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"host":                 host,
			"port":                 port,
			"server_name":          "localhost",
			"insecure_skip_verify": true,
			"min_days":             1,
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Output["ok"] != true {
		t.Fatalf("ok = %#v", res.Output["ok"])
	}
	if days, ok := res.Output["days_remaining"].(int); !ok || days < 1 {
		t.Fatalf("days_remaining = %#v", res.Output["days_remaining"])
	}
}

func TestDNSResolve(t *testing.T) {
	res, err := NewDNSResolve().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{"name": "localhost", "record_type": "A", "expected": []string{"127.0.0.1"}}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	records, ok := res.Output["records"].([]string)
	if !ok || len(records) == 0 {
		t.Fatalf("records = %#v", res.Output["records"])
	}
	if res.Output["record_type"] != "A" {
		t.Fatalf("record_type = %#v", res.Output["record_type"])
	}
	if res.Output["matched_expected"] != true {
		t.Fatalf("matched_expected = %#v records=%#v", res.Output["matched_expected"], records)
	}
	if res.Output["ips"] == nil || res.Output["resolver"] != "default" {
		t.Fatalf("dns output missing aliases/resolver: %#v", res.Output)
	}
}

func TestDNSResolveValidationAndExpectedMiss(t *testing.T) {
	_, err := NewDNSResolve().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{}},
	})
	if err == nil || !strings.Contains(err.Error(), "requires args.name") {
		t.Fatalf("expected missing name error, got %v", err)
	}

	_, err = NewDNSResolve().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{"name": "localhost", "record_type": "SRV"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported dns record_type") {
		t.Fatalf("expected unsupported record type error, got %v", err)
	}

	res, err := NewDNSResolve().Apply(context.Background(), modules.Request{
		Step: workflow.Step{Args: map[string]any{
			"name":        "localhost",
			"record_type": "A",
			"expected":    []any{"203.0.113.1"},
		}},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Output["matched_expected"] != false {
		t.Fatalf("matched_expected = %#v, want false", res.Output["matched_expected"])
	}
}

func selfSignedCert() (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(48 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return tls.X509KeyPair(certPEM, keyPEM)
}
