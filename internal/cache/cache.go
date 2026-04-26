package cache

import (
	"net/http"
	"strings"
	"sync"
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
		c.host.Log(logging.LogLevelDebug, "Cache miss: %s %s", method, url)
		return 0, nil, nil, false
	}
	if time.Now().After(e.expires) {
		c.host.Log(logging.LogLevelDebug, "Cache expired: %s %s", method, url)
		return 0, nil, nil, false
	}
	c.host.Log(logging.LogLevelDebug, "Cache hit: %s %s", method, url)
	return e.statusCode, e.header.Clone(), append([]byte(nil), e.body...), true
}

// Put stores a response for later retrieval. Caller is responsible for
// deciding whether the response is cacheable (see Cacheable).
func (c *Cache) Put(method, url string, statusCode int, header http.Header, body []byte) {
	c.mu.Lock()
	c.entries[key(method, url)] = entry{
		statusCode: statusCode,
		header:     header.Clone(),
		body:       append([]byte(nil), body...),
		expires:    time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
	c.host.Log(logging.LogLevelDebug, "Cached: %s %s (%d bytes)", method, url, len(body))
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
// Conservative: only GET responses with status 200, no Cache-Control:
// no-store on either side, and no Set-Cookie on the response.
func Cacheable(method string, reqHeader http.Header, statusCode int, respHeader http.Header) bool {
	if method != http.MethodGet {
		return false
	}
	if statusCode != http.StatusOK {
		return false
	}
	if hasNoStore(reqHeader.Values("Cache-Control")) {
		return false
	}
	if hasNoStore(respHeader.Values("Cache-Control")) {
		return false
	}
	if respHeader.Get("Set-Cookie") != "" {
		return false
	}
	return true
}

func hasNoStore(values []string) bool {
	for _, v := range values {
		for _, tok := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(tok), "no-store") {
				return true
			}
		}
	}
	return false
}
