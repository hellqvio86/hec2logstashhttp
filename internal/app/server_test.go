package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mockForwarder struct {
	lastPath string
	lastBody []byte
	lastMeta ForwardMeta
	err      error
}

func (m *mockForwarder) Forward(_ context.Context, path string, body []byte, meta ForwardMeta) error {
	m.lastPath = path
	m.lastBody = append([]byte(nil), body...)
	m.lastMeta = meta
	return m.err
}

func testConfig() Config {
	return Config{
		ListenAddr:      ":0",
		ForwardURL:      "http://127.0.0.1:18088/services/collector/event",
		Token:           "",
		RequestTimeout:  time.Second,
		ShutdownTimeout: 2 * time.Second,
		MaxBodyBytes:    1024,
		LogLevel:        slog.LevelError,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCollectSuccess(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("Authorization", "Splunk anything")
	req.Header.Set("User-Agent", "HomeAssistant/2026.2")
	req.RemoteAddr = "192.0.2.10:12345"
	req.Host = "example.test"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":0`) {
		t.Fatalf("expected code 0 body, got %s", w.Body.String())
	}
	if mf.lastPath != "/services/collector/event" {
		t.Fatalf("unexpected forward path: %s", mf.lastPath)
	}
	var got map[string]any
	if err := json.Unmarshal(mf.lastBody, &got); err != nil {
		t.Fatalf("forwarded body is not JSON: %v", err)
	}
	if got["message"] != "ok" {
		t.Fatalf("unexpected forwarded body: %s", string(mf.lastBody))
	}
	if mf.lastMeta.UserAgent != "HomeAssistant/2026.2" {
		t.Fatalf("unexpected forwarded user-agent: %s", mf.lastMeta.UserAgent)
	}
	if mf.lastMeta.AuthHeader != "Splunk anything" {
		t.Fatalf("unexpected auth header: %s", mf.lastMeta.AuthHeader)
	}
	if mf.lastMeta.ClientIP != "192.0.2.10" {
		t.Fatalf("unexpected client ip: %s", mf.lastMeta.ClientIP)
	}
	if mf.lastMeta.Host != "example.test" {
		t.Fatalf("unexpected host: %s", mf.lastMeta.Host)
	}
	if mf.lastMeta.Proto != "http" {
		t.Fatalf("unexpected proto: %s", mf.lastMeta.Proto)
	}
}

func TestCollectUnauthorized(t *testing.T) {
	cfg := testConfig()
	cfg.Token = "topsecret"
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("Authorization", "Splunk wrong")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", w.Code)
	}
	if mf.lastPath != "" {
		t.Fatalf("expected no forwarding, got path %s", mf.lastPath)
	}
}

func TestCollectForwardFailure(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{err: errors.New("boom")}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":9`) {
		t.Fatalf("expected code 9 body, got %s", w.Body.String())
	}
}

func TestCollectMethodNotAllowed(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodGet, "/services/collector/event", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 got %d", w.Code)
	}
}

func TestCollectNoData(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader("   "))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":5`) {
		t.Fatalf("expected code 5 body, got %s", w.Body.String())
	}
	if mf.lastPath != "" {
		t.Fatalf("expected no forwarding for no-data check, got %s", mf.lastPath)
	}
}

func TestCollectInvalidHECPayload(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"message":"not-hec"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"code":6`) {
		t.Fatalf("expected code 6 body, got %s", w.Body.String())
	}
	if mf.lastPath != "" {
		t.Fatalf("expected no forwarding for invalid payload, got %s", mf.lastPath)
	}
}

func TestCollectClientIPFromForwardedHeader(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("Forwarded", `for=198.51.100.7;proto=https, for=127.0.0.1`)
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	req.RemoteAddr = "127.0.0.1:34567"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if mf.lastMeta.ClientIP != "198.51.100.7" {
		t.Fatalf("unexpected client ip from forwarded header: %s", mf.lastMeta.ClientIP)
	}
}

func TestCollectClientIPFromXForwardedForHeader(t *testing.T) {
	cfg := testConfig()
	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok"}`))
	req.Header.Set("X-Forwarded-For", "198.51.100.8, 127.0.0.1")
	req.Header.Set("X-Real-IP", "10.0.0.5")
	req.RemoteAddr = "127.0.0.1:34567"
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if mf.lastMeta.ClientIP != "198.51.100.8" {
		t.Fatalf("unexpected client ip from x-forwarded-for header: %s", mf.lastMeta.ClientIP)
	}
}

func TestCollectWithTokenInputProfile(t *testing.T) {
	cfg := testConfig()
	cfg.Inputs = []InputProfile{
		{
			Token:                        "profile-token",
			Name:                         "homeassistant-prod",
			Route:                        "default",
			ForwardURL:                   "http://127.0.0.1:18088/services/collector/event",
			DataStream:                   "logs-homeassistant",
			Namespace:                    "prod",
			DefaultSourcetype:            "homeassistant:event",
			DefaultSource:                "homeassistant",
			AllowEventSourcetypeOverride: false,
			AllowEventSourceOverride:     true,
		},
	}
	cfg.RejectUnknown = true

	mf := &mockForwarder{}
	h := newHandler(cfg, testLogger(), mf)

	req := httptest.NewRequest(http.MethodPost, "/services/collector/event", strings.NewReader(`{"event":"ok","sourcetype":"custom:st","source":"sensor-a"}`))
	req.Header.Set("Authorization", "Splunk profile-token")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d", w.Code)
	}
	if mf.lastMeta.ForwardURL != "http://127.0.0.1:18088/services/collector/event" {
		t.Fatalf("unexpected forward url: %s", mf.lastMeta.ForwardURL)
	}

	var got map[string]any
	if err := json.Unmarshal(mf.lastBody, &got); err != nil {
		t.Fatalf("forwarded body is not JSON: %v", err)
	}
	if got["hec_token_name"] != "homeassistant-prod" {
		t.Fatalf("missing hec_token_name enrichment: %#v", got)
	}
	if got["hec_route"] != "default" {
		t.Fatalf("missing hec_route enrichment: %#v", got)
	}
	if got["hec_datastream"] != "logs-homeassistant" {
		t.Fatalf("missing hec_datastream enrichment: %#v", got)
	}
	if got["hec_namespace"] != "prod" {
		t.Fatalf("missing hec_namespace enrichment: %#v", got)
	}
	if got["hec_sourcetype"] != "homeassistant:event" {
		t.Fatalf("expected profile-enforced sourcetype, got %#v", got["hec_sourcetype"])
	}
	if got["hec_source"] != "sensor-a" {
		t.Fatalf("expected event source override to be preserved, got %#v", got["hec_source"])
	}
}

func generateTestCertificate(t *testing.T, dir string) (string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")

	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		t.Fatalf("encode cert: %v", err)
	}
	_ = certOut.Close()

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		t.Fatalf("encode key: %v", err)
	}
	_ = keyOut.Close()

	return certPath, keyPath
}

func TestBuildTLSConfigValidation(t *testing.T) {
	// TLS disabled returns nil config without error
	cfgDisabled := Config{TLSEnabled: false}
	tlsCfg, err := buildTLSConfig(cfgDisabled)
	if err != nil || tlsCfg != nil {
		t.Fatalf("expected nil tlsConfig and nil err when disabled, got %v, %v", tlsCfg, err)
	}

	// Missing cert file
	cfgNoCert := Config{TLSEnabled: true, TLSKeyFile: "key.pem"}
	if _, err := buildTLSConfig(cfgNoCert); err == nil {
		t.Fatal("expected error when cert file is missing")
	}

	// Missing key file
	cfgNoKey := Config{TLSEnabled: true, TLSCertFile: "cert.pem"}
	if _, err := buildTLSConfig(cfgNoKey); err == nil {
		t.Fatal("expected error when key file is missing")
	}

	// Non-existent cert/key files
	cfgNonExistent := Config{TLSEnabled: true, TLSCertFile: "nope.pem", TLSKeyFile: "nope.key"}
	if _, err := buildTLSConfig(cfgNonExistent); err == nil {
		t.Fatal("expected error for non-existent cert/key files")
	}

	// Valid cert & key
	dir := t.TempDir()
	certPath, keyPath := generateTestCertificate(t, dir)

	cfgValid := Config{
		TLSEnabled:    true,
		TLSCertFile:   certPath,
		TLSKeyFile:    keyPath,
		TLSMinVersion: "1.3",
	}
	tlsCfgValid, err := buildTLSConfig(cfgValid)
	if err != nil {
		t.Fatalf("expected no error for valid cert/key, got %v", err)
	}
	if tlsCfgValid == nil || tlsCfgValid.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected MinVersion TLS1.3, got %v", tlsCfgValid)
	}

	// Invalid min version
	cfgInvalidVer := Config{
		TLSEnabled:    true,
		TLSCertFile:   certPath,
		TLSKeyFile:    keyPath,
		TLSMinVersion: "1.1",
	}
	if _, err := buildTLSConfig(cfgInvalidVer); err == nil {
		t.Fatal("expected error for unsupported min version 1.1")
	}
}

func TestServerHTTPSEndToEnd(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCertificate(t, dir)

	// Bind a local ephemeral port first to get an address
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ephemeral port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cfg := Config{
		ListenAddr:      addr,
		ForwardURL:      "http://127.0.0.1:18088/services/collector/event",
		RequestTimeout:  time.Second,
		ShutdownTimeout: 2 * time.Second,
		MaxBodyBytes:    1024,
		LogLevel:        slog.LevelError,
		TLSEnabled:      true,
		TLSCertFile:     certPath,
		TLSKeyFile:      keyPath,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- srv.ListenAndServe()
	}()

	// Trust the self-signed cert in client
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert pem: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("failed to append test cert to pool")
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: roots,
			},
		},
		Timeout: 3 * time.Second,
	}

	// Poll until server is ready
	var resp *http.Response
	healthURL := fmt.Sprintf("https://%s/healthz", addr)
	for i := 0; i < 30; i++ {
		resp, err = client.Get(healthURL)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to connect to HTTPS healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from https healthz, got %d", resp.StatusCode)
	}

	var res HECResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if res.Text != "ok" || res.Code != 0 {
		t.Fatalf("unexpected healthz response: %+v", res)
	}

	_ = srv.Shutdown(context.Background())
	select {
	case err := <-serverDone:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("unexpected server exit error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop in time")
	}
}

func writePEM(t *testing.T, path string, blockType string, data []byte) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file %s: %v", path, err)
	}
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: data}); err != nil {
		t.Fatalf("encode pem %s: %v", path, err)
	}
	_ = f.Close()
}

func generateCAAndCerts(t *testing.T, dir string) (string, string, string, string, string) {
	t.Helper()
	caPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ca key: %v", err)
	}
	caTemplate := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "Test Root CA",
			Organization: []string{"Test CA Org"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caBytes, err := x509.CreateCertificate(rand.Reader, &caTemplate, &caTemplate, &caPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("create ca cert: %v", err)
	}
	caCertPath := filepath.Join(dir, "ca.pem")
	writePEM(t, caCertPath, "CERTIFICATE", caBytes)

	// Server cert signed by CA
	serverPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate server key: %v", err)
	}
	serverTemplate := x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject: pkix.Name{
			CommonName: "localhost",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	serverBytes, err := x509.CreateCertificate(rand.Reader, &serverTemplate, &caTemplate, &serverPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("create server cert: %v", err)
	}
	serverCertPath := filepath.Join(dir, "server-cert.pem")
	serverKeyPath := filepath.Join(dir, "server-key.pem")
	writePEM(t, serverCertPath, "CERTIFICATE", serverBytes)
	sKeyBytes, err := x509.MarshalECPrivateKey(serverPriv)
	if err != nil {
		t.Fatalf("marshal server key: %v", err)
	}
	writePEM(t, serverKeyPath, "EC PRIVATE KEY", sKeyBytes)

	// Client cert signed by CA
	clientPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientTemplate := x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject: pkix.Name{
			CommonName: "test-client",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	clientBytes, err := x509.CreateCertificate(rand.Reader, &clientTemplate, &caTemplate, &clientPriv.PublicKey, caPriv)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}
	clientCertPath := filepath.Join(dir, "client-cert.pem")
	clientKeyPath := filepath.Join(dir, "client-key.pem")
	writePEM(t, clientCertPath, "CERTIFICATE", clientBytes)
	cKeyBytes, err := x509.MarshalECPrivateKey(clientPriv)
	if err != nil {
		t.Fatalf("marshal client key: %v", err)
	}
	writePEM(t, clientKeyPath, "EC PRIVATE KEY", cKeyBytes)

	return caCertPath, serverCertPath, serverKeyPath, clientCertPath, clientKeyPath
}

func TestBuildTLSConfigClientCA(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCertificate(t, dir)

	// Client CA file missing
	cfgNoCA := Config{
		TLSEnabled:      true,
		TLSCertFile:     certPath,
		TLSKeyFile:      keyPath,
		TLSClientCAFile: filepath.Join(dir, "nonexistent-ca.pem"),
	}
	if _, err := buildTLSConfig(cfgNoCA); err == nil {
		t.Fatal("expected error when client CA file does not exist")
	}

	// Client CA file invalid PEM
	invalidCAPath := filepath.Join(dir, "invalid-ca.pem")
	if err := os.WriteFile(invalidCAPath, []byte("NOT A PEM CERTIFICATE"), 0o600); err != nil {
		t.Fatalf("write invalid ca: %v", err)
	}
	cfgInvalidCA := Config{
		TLSEnabled:      true,
		TLSCertFile:     certPath,
		TLSKeyFile:      keyPath,
		TLSClientCAFile: invalidCAPath,
	}
	if _, err := buildTLSConfig(cfgInvalidCA); err == nil {
		t.Fatal("expected error for invalid PEM in client CA")
	}

	// Valid Client CA
	caCertPath, _, _, _, _ := generateCAAndCerts(t, dir)
	cfgValidCA := Config{
		TLSEnabled:      true,
		TLSCertFile:     certPath,
		TLSKeyFile:      keyPath,
		TLSClientCAFile: caCertPath,
	}
	tlsCfg, err := buildTLSConfig(cfgValidCA)
	if err != nil {
		t.Fatalf("expected no error for valid client CA, got %v", err)
	}
	if tlsCfg.ClientCAs == nil {
		t.Fatal("expected non-nil ClientCAs")
	}
	if tlsCfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Fatalf("expected RequireAndVerifyClientCert, got %v", tlsCfg.ClientAuth)
	}
}

func TestServerHTTPSCollectEvent(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := generateTestCertificate(t, dir)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ephemeral port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cfg := Config{
		ListenAddr:      addr,
		ForwardURL:      "http://127.0.0.1:18088/services/collector/event",
		Token:           "secure-token-123",
		RequestTimeout:  time.Second,
		ShutdownTimeout: 2 * time.Second,
		MaxBodyBytes:    1024,
		LogLevel:        slog.LevelError,
		TLSEnabled:      true,
		TLSCertFile:     certPath,
		TLSKeyFile:      keyPath,
	}

	mf := &mockForwarder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("buildTLSConfig failed: %v", err)
	}

	srv := &Server{
		cfg: cfg,
		httpServer: &http.Server{
			Addr:              cfg.ListenAddr,
			Handler:           newHandler(cfg, logger, mf),
			TLSConfig:         tlsCfg,
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       60 * time.Second,
		},
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- srv.ListenAndServe()
	}()

	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("read cert pem: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(certPEM)

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: roots,
			},
		},
		Timeout: 3 * time.Second,
	}

	// Test POST /services/collector/event over HTTPS
	eventURL := fmt.Sprintf("https://%s/services/collector/event", addr)
	payload := strings.NewReader(`{"event":"temperature: 21.5","sourcetype":"homeassistant:event"}`)
	req, err := http.NewRequest(http.MethodPost, eventURL, payload)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Authorization", "Splunk secure-token-123")
	req.Header.Set("User-Agent", "HomeAssistant/2026.3")

	var resp *http.Response
	for i := 0; i < 30; i++ {
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("failed to post event over HTTPS: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if mf.lastMeta.Proto != "https" {
		t.Fatalf("expected proto https, got %q", mf.lastMeta.Proto)
	}
	if mf.lastMeta.AuthHeader != "Splunk secure-token-123" {
		t.Fatalf("expected auth header preserved, got %q", mf.lastMeta.AuthHeader)
	}

	_ = srv.Shutdown(context.Background())
	_ = <-serverDone
}

func TestServerHTTPSmTLS(t *testing.T) {
	dir := t.TempDir()
	caCertPath, serverCertPath, serverKeyPath, clientCertPath, clientKeyPath := generateCAAndCerts(t, dir)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ephemeral port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	cfg := Config{
		ListenAddr:      addr,
		ForwardURL:      "http://127.0.0.1:18088/services/collector/event",
		RequestTimeout:  time.Second,
		ShutdownTimeout: 2 * time.Second,
		MaxBodyBytes:    1024,
		LogLevel:        slog.LevelError,
		TLSEnabled:      true,
		TLSCertFile:     serverCertPath,
		TLSKeyFile:      serverKeyPath,
		TLSClientCAFile: caCertPath,
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := NewServer(cfg, logger)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	serverDone := make(chan error, 1)
	go func() {
		serverDone <- srv.ListenAndServe()
	}()

	caPEM, _ := os.ReadFile(caCertPath)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(caPEM)

	// 1. Client without certificate -> handshake failure
	clientNoCert := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: roots,
			},
		},
		Timeout: 2 * time.Second,
	}
	healthURL := fmt.Sprintf("https://%s/healthz", addr)
	// Retry connection until server is listening
	var connected bool
	for i := 0; i < 30; i++ {
		_, reqErr := clientNoCert.Get(healthURL)
		if reqErr != nil {
			// Check if it's TLS handshake error (expected) or connection refused
			if strings.Contains(reqErr.Error(), "tls:") || strings.Contains(reqErr.Error(), "certificate") {
				connected = true
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if !connected {
		t.Fatal("expected TLS handshake failure for client without certificate")
	}

	// 2. Client WITH valid certificate -> succeeds
	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		t.Fatalf("load client cert pair: %v", err)
	}

	clientWithCert := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:      roots,
				Certificates: []tls.Certificate{clientCert},
			},
		},
		Timeout: 3 * time.Second,
	}

	resp, err := clientWithCert.Get(healthURL)
	if err != nil {
		t.Fatalf("expected mTLS get to succeed, got: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from mTLS healthz, got %d", resp.StatusCode)
	}

	_ = srv.Shutdown(context.Background())
	_ = <-serverDone
}
