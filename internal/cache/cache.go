package cache

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"httpServer/internal/logging"
)

// Host is the narrow interface the cache needs from its caller. Defined
// here (rather than in the proxy package) so cache stays import-cycle free.
type Host interface {
	Log(level logging.LogLevel, format string, args ...interface{})
	GetCachingTTL() time.Duration
}

type Cache struct {
	host    Host
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]entry
	stop    chan struct{}

	hits   atomic.Int64
	misses atomic.Int64
}

// New starts a Cache and a background sweeper that purges expired entries.
func New(host Host) *Cache {
	ttl := host.GetCachingTTL()
	if ttl <= 0 {
		ttl = time.Hour
	}
	c := &Cache{
		host:    host,
		ttl:     ttl,
		entries: make(map[string]entry),
		stop:    make(chan struct{}),
	}
	host.Log(logging.LogLevelDebug, "Starting Cache (ttl=%s)", c.ttl)
	go c.sweep()
	return c
}

func (c *Cache) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

// Hits returns the running count of cache hits served by Get since the cache
// started.
func (c *Cache) Hits() int64 { return c.hits.Load() }

// Misses returns the running count of cache misses served by Get since the
// cache started (including expired entries).
func (c *Cache) Misses() int64 { return c.misses.Load() }

func key(method, url string) string {
	return method + " " + url
}

// Get returns a cached response for the request, or ok=false on miss/expiry.
// Caller decides whether to call Get based on method, etc.
func (c *Cache) Get(method, url string) (statusCode int, header http.Header, body []byte, ok bool) {
	c.mu.RLock()
	e, found := c.entries[key(method, url)]
	c.mu.RUnlock()
	if !found {
		c.misses.Add(1)
		c.host.Log(logging.LogLevelDebug, "Cache miss: %s %s", method, url)
		return 0, nil, nil, false
	}
	if time.Now().After(e.expires) {
		c.misses.Add(1)
		c.host.Log(logging.LogLevelDebug, "Cache expired: %s %s", method, url)
		return 0, nil, nil, false
	}
	c.hits.Add(1)
	c.host.Log(logging.LogLevelDebug, "Cache hit: %s %s", method, url)
	return e.statusCode, e.header.Clone(), append([]byte(nil), e.body...), true
}

// Put stores a response for later retrieval. Caller is responsible for
// deciding whether the response is cacheable (see Cacheable). The per-entry
// TTL respects Cache-Control: s-maxage / max-age on the response, falling
// back to the cache's configured default TTL.
func (c *Cache) Put(method, url string, statusCode int, header http.Header, body []byte) {
	ttl := c.ttl
	if d, ok := parseMaxAge(header.Values("Cache-Control")); ok && d > 0 {
		ttl = d
	}
	c.mu.Lock()
	c.entries[key(method, url)] = entry{
		statusCode: statusCode,
		header:     header.Clone(),
		body:       append([]byte(nil), body...),
		expires:    time.Now().Add(ttl),
	}
	c.mu.Unlock()
	c.host.Log(logging.LogLevelDebug, "Cached: %s %s (%d bytes, ttl=%s)", method, url, len(body), ttl)
}

func (c *Cache) sweep() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			for k, e := range c.entries {
				if now.After(e.expires) {
					delete(c.entries, k)
				}
			}
			c.mu.Unlock()
		}
	}
}

// Cacheable reports whether a request/response pair is safe to cache.
// Conservative: GET only, status 200 only, no Cache-Control no-store/
// no-cache/private on either side, no Set-Cookie, and refuses max-age=0.
func Cacheable(method string, reqHeader http.Header, statusCode int, respHeader http.Header) bool {
	if method != http.MethodGet {
		return false
	}
	if statusCode != http.StatusOK {
		return false
	}
	if hasDirective(reqHeader.Values("Cache-Control"), "no-store", "no-cache") {
		return false
	}
	respCC := respHeader.Values("Cache-Control")
	if hasDirective(respCC, "no-store", "no-cache", "private") {
		return false
	}
	if respHeader.Get("Set-Cookie") != "" {
		return false
	}
	if d, ok := parseMaxAge(respCC); ok && d <= 0 {
		return false
	}
	return true
}

// parseMaxAge picks s-maxage when present (shared cache directive),
// otherwise max-age. Returns ok=false if neither directive is set.
func parseMaxAge(values []string) (time.Duration, bool) {
	var maxAge, sMaxAge time.Duration
	var hasMA, hasSM bool
	for _, v := range values {
		for _, tok := range strings.Split(v, ",") {
			tok = strings.TrimSpace(tok)
			switch {
			case strings.HasPrefix(strings.ToLower(tok), "s-maxage="):
				if n, err := strconv.Atoi(tok[len("s-maxage="):]); err == nil {
					sMaxAge = time.Duration(n) * time.Second
					hasSM = true
				}
			case strings.HasPrefix(strings.ToLower(tok), "max-age="):
				if n, err := strconv.Atoi(tok[len("max-age="):]); err == nil {
					maxAge = time.Duration(n) * time.Second
					hasMA = true
				}
			}
		}
	}
	if hasSM {
		return sMaxAge, true
	}
	if hasMA {
		return maxAge, true
	}
	return 0, false
}

func hasDirective(values []string, names ...string) bool {
	for _, v := range values {
		for _, tok := range strings.Split(v, ",") {
			tok = strings.TrimSpace(tok)
			if eq := strings.IndexByte(tok, '='); eq >= 0 {
				tok = tok[:eq]
			}
			for _, n := range names {
				if strings.EqualFold(tok, n) {
					return true
				}
			}
		}
	}
	return false
}
