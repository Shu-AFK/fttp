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
	"sync"
	"sync/atomic"
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
	srv, _ := startCountingBackend(t)
	return srv
}

// startCountingBackend is like startBackend but exposes a hit counter so tests
// can assert how many times the backend was actually invoked (useful for cache
// verification).
func startCountingBackend(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	srv, hits, _ := startConfigurableBackend(t)
	return srv, hits
}

// startConfigurableBackend returns a server, a hit counter, and a per-request
// header map that the backend will copy into its response. Lets cache tests
// inject Cache-Control directives.
func startConfigurableBackend(t *testing.T) (*httptest.Server, *atomic.Int32, *sync.Map) {
	t.Helper()
	var hits atomic.Int32
	var respHeaders sync.Map // path -> http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		if v, ok := respHeaders.Load(r.URL.Path); ok {
			for k, vs := range v.(http.Header) {
				for _, val := range vs {
					w.Header().Add(k, val)
				}
			}
		}
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
	return srv, &hits, &respHeaders
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
// The proxy is automatically Stop()'d on test cleanup.
// cacheTTL > 0 enables the cache with that TTL (in seconds).
func startProxy(t *testing.T, backend string, cacheTTL int) int {
	t.Helper()
	_, port := startProxyHandle(t, backend, cacheTTL)
	return port
}

// startProxyHandle is like startProxy but returns the *proxy.Proxy too, plus a
// channel that fires once Start has returned. Used by the shutdown test.
func startProxyHandle(t *testing.T, backend string, cacheTTL int) (*proxy.Proxy, int) {
	t.Helper()
	dir := t.TempDir()
	certPath, keyPath := writeSelfSignedCert(t, dir)

	port := freePort(t)
	configPath := filepath.Join(dir, "config.yaml")
	logPath := filepath.Join(dir, "proxy.log")
	cachingEnabled := cacheTTL > 0
	configTTL := cacheTTL
	if configTTL <= 0 {
		configTTL = 3600
	}
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
  enabled: %t
  ttl: %d
blacklist: []
logger:
  level: "warn"
  file: %q
`, port, backend, backend, cachingEnabled, configTTL, logPath)
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
	t.Cleanup(p.Stop)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return p, port
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("proxy did not start listening on :%d", port)
	return nil, 0
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
	port := startProxy(t, backend.URL, 0)
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

func TestCaching(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped under -short")
	}

	backend, hits := startCountingBackend(t)
	port := startProxy(t, backend.URL, 60)
	base := fmt.Sprintf("https://127.0.0.1:%d", port)
	client := http11Client()

	t.Run("repeated_get_hits_backend_once", func(t *testing.T) {
		hits.Store(0)
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest("GET", base+"/api/v1/cached", nil)
			code, echo := do(t, client, req)
			if code != 200 {
				t.Fatalf("status=%d", code)
			}
			if echo.Path != "/api/v1/cached" {
				t.Errorf("path=%q", echo.Path)
			}
		}
		if got := hits.Load(); got != 1 {
			t.Errorf("backend hits=%d, want 1 (cache should serve repeats)", got)
		}
	})

	t.Run("post_is_not_cached", func(t *testing.T) {
		hits.Store(0)
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest("POST", base+"/api/v1/post-cached", strings.NewReader(""))
			code, _ := do(t, client, req)
			if code != 200 {
				t.Fatalf("status=%d", code)
			}
		}
		if got := hits.Load(); got != 3 {
			t.Errorf("backend hits=%d, want 3 (POST must not be cached)", got)
		}
	})

	t.Run("different_paths_are_separate_keys", func(t *testing.T) {
		hits.Store(0)
		for _, path := range []string{"/api/v1/a", "/api/v1/b", "/api/v1/a", "/api/v1/b"} {
			req, _ := http.NewRequest("GET", base+path, nil)
			code, _ := do(t, client, req)
			if code != 200 {
				t.Fatalf("status=%d for %s", code, path)
			}
		}
		if got := hits.Load(); got != 2 {
			t.Errorf("backend hits=%d, want 2 (one miss per distinct path)", got)
		}
	})
}

func TestCacheControl(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped under -short")
	}

	backend, hits, headers := startConfigurableBackend(t)
	p, port := startProxyHandle(t, backend.URL, 60)
	base := fmt.Sprintf("https://127.0.0.1:%d", port)
	client := http11Client()

	t.Run("max_age_zero_not_cached", func(t *testing.T) {
		hits.Store(0)
		headers.Store("/api/v1/no-cache-zero", http.Header{"Cache-Control": []string{"max-age=0"}})
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest("GET", base+"/api/v1/no-cache-zero", nil)
			if code, _ := do(t, client, req); code != 200 {
				t.Fatalf("status=%d", code)
			}
		}
		if got := hits.Load(); got != 3 {
			t.Errorf("backend hits=%d, want 3 (max-age=0 must not be cached)", got)
		}
	})

	t.Run("no_cache_directive_not_cached", func(t *testing.T) {
		hits.Store(0)
		headers.Store("/api/v1/private", http.Header{"Cache-Control": []string{"no-cache"}})
		for i := 0; i < 3; i++ {
			req, _ := http.NewRequest("GET", base+"/api/v1/private", nil)
			if code, _ := do(t, client, req); code != 200 {
				t.Fatalf("status=%d", code)
			}
		}
		if got := hits.Load(); got != 3 {
			t.Errorf("backend hits=%d, want 3 (no-cache must not be cached)", got)
		}
	})

	t.Run("max_age_short_ttl_expires", func(t *testing.T) {
		hits.Store(0)
		headers.Store("/api/v1/short", http.Header{"Cache-Control": []string{"max-age=1"}})
		req, _ := http.NewRequest("GET", base+"/api/v1/short", nil)
		if code, _ := do(t, client, req); code != 200 {
			t.Fatalf("status=%d", code)
		}
		if code, _ := do(t, client, req); code != 200 {
			t.Fatalf("status=%d", code)
		}
		if got := hits.Load(); got != 1 {
			t.Errorf("backend hits=%d, want 1 (within max-age window)", got)
		}
		// Wait past the per-entry TTL set by Cache-Control: max-age=1.
		time.Sleep(1100 * time.Millisecond)
		if code, _ := do(t, client, req); code != 200 {
			t.Fatalf("status=%d", code)
		}
		if got := hits.Load(); got != 2 {
			t.Errorf("backend hits=%d, want 2 (max-age expired)", got)
		}
	})

	t.Run("hit_miss_counters", func(t *testing.T) {
		// Reset baseline.
		hBefore, mBefore := p.CacheStats()
		req, _ := http.NewRequest("GET", base+"/api/v1/counter-test", nil)
		_, _ = do(t, client, req) // first call: miss
		_, _ = do(t, client, req) // second call: hit
		_, _ = do(t, client, req) // third call: hit
		hAfter, mAfter := p.CacheStats()
		if h := hAfter - hBefore; h != 2 {
			t.Errorf("hits delta=%d, want 2", h)
		}
		if m := mAfter - mBefore; m != 1 {
			t.Errorf("misses delta=%d, want 1", m)
		}
	})
}

func TestGracefulShutdown(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test skipped under -short")
	}

	backend := startBackend(t)

	// Boot proxy manually so we can observe Start's return value.
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
add_header: {}
caching:
  enabled: false
  ttl: 3600
blacklist: []
logger:
  level: "warn"
  file: %q
`, port, backend.URL, logPath)
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	certs, err := proxy.LoadCertificates(certPath, keyPath)
	if err != nil {
		t.Fatalf("load certs: %v", err)
	}

	p := proxy.New(configPath)
	startErr := make(chan error, 1)
	go func() { startErr <- p.Start(certs) }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c, derr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
		if derr == nil {
			_ = c.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Confirm proxy is actually serving.
	client := http11Client()
	req, _ := http.NewRequest("GET", fmt.Sprintf("https://127.0.0.1:%d/api/v1", port), nil)
	if code, _ := do(t, client, req); code != 200 {
		t.Fatalf("pre-shutdown request status=%d", code)
	}

	// Idle close so the connection from the keep-alive pool doesn't hold
	// the proxy open past Stop.
	client.CloseIdleConnections()

	stoppedAt := time.Now()
	p.Stop()

	select {
	case err := <-startErr:
		if err != nil {
			t.Errorf("Start returned %v after Stop", err)
		}
		if elapsed := time.Since(stoppedAt); elapsed > 2*time.Second {
			t.Errorf("Start took %s to return after Stop, want <2s", elapsed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s after Stop")
	}

	// Stop must be safe to call again.
	p.Stop()

	// Port should now refuse connections.
	if _, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond); err == nil {
		t.Errorf("port :%d still accepts connections after Stop", port)
	}
}
