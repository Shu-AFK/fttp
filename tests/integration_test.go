package integration_test

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"httpServer/internal/proxy"
)

type echoResponse struct {
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Query   string      `json:"query"`
	Headers http.Header `json:"headers"`
	Body    string      `json:"body"`
}

// startBackend starts a local HTTP/1.1 echo server that returns request shape as JSON.
func startBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(echoResponse{
			Method:  r.Method,
			Path:    r.URL.Path,
			Query:   r.URL.RawQuery,
			Headers: r.Header,
			Body:    string(body),
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeSelfSignedCert writes a fresh in-memory self-signed cert/key pair to dir,
// returning the cert and key file paths.
func writeSelfSignedCert(t *testing.T, dir string) (certPath, keyPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{Organization: []string{"fttp-test"}, CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")

	certFile, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("pem encode cert: %v", err)
	}
	_ = certFile.Close()

	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		t.Fatalf("pem encode key: %v", err)
	}
	_ = keyFile.Close()
	return certPath, keyPath
}

// freePort allocates a TCP port the OS isn't using.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// startProxy boots a Proxy in a goroutine routed at backend, returning its port.
func startProxy(t *testing.T, backend string) int {
	t.Helper()
	dir := t.TempDir()
	certPath, keyPath := writeSelfSignedCert(t, dir)

	port := freePort(t)
	configPath := filepath.Join(dir, "config.yaml")
	logPath := filepath.Join(dir, "proxy.log")
	config := fmt.Sprintf(`server:
  port: %d
  routes:
    - path: "/api/v1"
      host: %q
      target_path: "/api/v1"
    - path: "/api/v2"
      host: %q
      target_path: "/api/v2"
add_header:
  X-Test: ["test-value"]
caching:
  enabled: false
  ttl: 3600
blacklist: []
logger:
  level: "warn"
  file: %q
`, port, backend, backend, logPath)
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	certs, err := proxy.LoadCertificates(certPath, keyPath)
	if err != nil {
		t.Fatalf("load certs: %v", err)
	}

	p := proxy.New(configPath)
	go func() {
		_ = p.Start(certs)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return port
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("proxy did not start listening on :%d", port)
	return 0
}

func http11Client() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
				NextProtos:         []string{"http/1.1"},
			},
			ForceAttemptHTTP2: false,
			TLSNextProto:      map[string]func(string, *tls.Conn) http.RoundTripper{},
		},
	}
}

func http2Client() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http2.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func do(t *testing.T, client *http.Client, req *http.Request) (int, echoResponse) {
	t.Helper()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var out echoResponse
	if len(body) > 0 && resp.Header.Get("Content-Type") == "application/json" {
		if err := json.Unmarshal(body, &out); err != nil {
			t.Fatalf("unmarshal echo body %q: %v", body, err)
		}
	}
	return resp.StatusCode, out
}

func TestReverseProxy(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped under -short")
	}

	backend := startBackend(t)
	port := startProxy(t, backend.URL)
	base := fmt.Sprintf("https://127.0.0.1:%d", port)

	clients := map[string]*http.Client{
		"http1.1": http11Client(),
		"http2":   http2Client(),
	}

	for name, client := range clients {
		t.Run(name, func(t *testing.T) {
			t.Run("exact_match", func(t *testing.T) {
				req, _ := http.NewRequest("GET", base+"/api/v1", nil)
				code, echo := do(t, client, req)
				if code != 200 {
					t.Fatalf("status=%d", code)
				}
				if echo.Path != "/api/v1" {
					t.Errorf("path=%q", echo.Path)
				}
				if echo.Headers.Get("X-Test") != "test-value" {
					t.Errorf("X-Test missing: %v", echo.Headers)
				}
				if got := echo.Headers.Get("X-Forwarded-For"); got == "" {
					t.Errorf("X-Forwarded-For empty")
				}
			})

			t.Run("subpath_with_query", func(t *testing.T) {
				req, _ := http.NewRequest("GET", base+"/api/v1/users?id=42&name=foo", nil)
				code, echo := do(t, client, req)
				if code != 200 {
					t.Fatalf("status=%d", code)
				}
				if echo.Path != "/api/v1/users" {
					t.Errorf("path=%q", echo.Path)
				}
				if echo.Query != "id=42&name=foo" {
					t.Errorf("query=%q", echo.Query)
				}
			})

			t.Run("post_with_body", func(t *testing.T) {
				req, _ := http.NewRequest("POST", base+"/api/v2/items", strings.NewReader(`{"k":"v"}`))
				req.Header.Set("Content-Type", "application/json")
				code, echo := do(t, client, req)
				if code != 200 {
					t.Fatalf("status=%d", code)
				}
				if echo.Body != `{"k":"v"}` {
					t.Errorf("body=%q", echo.Body)
				}
				if echo.Headers.Get("Content-Type") != "application/json" {
					t.Errorf("Content-Type lost: %v", echo.Headers)
				}
			})

			t.Run("xff_chain", func(t *testing.T) {
				req, _ := http.NewRequest("GET", base+"/api/v1/xff", nil)
				req.Header.Set("X-Forwarded-For", "10.0.0.5")
				code, echo := do(t, client, req)
				if code != 200 {
					t.Fatalf("status=%d", code)
				}
				if !strings.HasPrefix(echo.Headers.Get("X-Forwarded-For"), "10.0.0.5,") {
					t.Errorf("X-Forwarded-For chain missing: %q", echo.Headers.Get("X-Forwarded-For"))
				}
			})

			if name == "http1.1" {
				// Go's http2 client refuses to send a Connection header at all
				// (RFC 7540 §8.1.2.2 forbids it), so hop-by-hop stripping can
				// only be exercised over HTTP/1.1.
				t.Run("hop_headers_stripped", func(t *testing.T) {
					req, _ := http.NewRequest("GET", base+"/api/v1/check", nil)
					req.Header.Set("Connection", "close, X-Hop-Test")
					req.Header.Set("X-Hop-Test", "should-be-stripped")
					code, echo := do(t, client, req)
					if code != 200 {
						t.Fatalf("status=%d", code)
					}
					if echo.Headers.Get("X-Hop-Test") != "" {
						t.Errorf("hop header leaked: %q", echo.Headers.Get("X-Hop-Test"))
					}
				})
			}

			t.Run("unknown_path_404", func(t *testing.T) {
				req, _ := http.NewRequest("GET", base+"/no-such-path", nil)
				code, _ := do(t, client, req)
				if code != 404 {
					t.Errorf("status=%d, want 404", code)
				}
			})
		})
	}
}
