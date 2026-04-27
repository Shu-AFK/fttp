package cache

import (
	"bytes"
	"context"
	"net/http"
	"time"

	"httpServer/internal/logging"
)

// cacheStatusKey is the context key used by Middleware to flag the cache
// outcome for downstream loggers.
type cacheStatusKey struct{}

// CacheStatus retrieves "hit" or "miss" from the request context, or "" if
// the request didn't pass through the cache middleware.
func CacheStatus(r *http.Request) string {
	s, _ := r.Context().Value(cacheStatusKey{}).(string)
	return s
}

// Middleware returns an http handler that serves cached responses on hit,
// otherwise delegates to next and stores the response when it is Cacheable.
// A nil *Cache makes the middleware a no-op pass-through.
func Middleware(c *Cache, next http.HandlerFunc) http.HandlerFunc {
	if c == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.String()

		if r.Method == http.MethodGet {
			start := time.Now()
			if status, header, body, ok := c.Get(r.Method, key); ok {
				for name, vs := range header {
					for _, v := range vs {
						w.Header().Add(name, v)
					}
				}
				w.WriteHeader(status)
				_, _ = w.Write(body)
				c.host.Log(logging.LogLevelInfo,
					"request method=%s path=%q status=%d cache=hit dur=%s",
					r.Method, r.URL.Path, status, time.Since(start))
				return
			}
		}

		ctx := context.WithValue(r.Context(), cacheStatusKey{}, "miss")
		rec := newRecorder(w)
		next(rec, r.WithContext(ctx))

		if Cacheable(r.Method, r.Header, rec.statusCode, rec.Header()) {
			c.Put(r.Method, key, rec.statusCode, rec.Header(), rec.body.Bytes())
		}
	}
}

// recorder buffers the response written by a handler so the middleware
// can inspect status/body for caching, while still streaming bytes through
// to the underlying writer.
type recorder struct {
	http.ResponseWriter
	statusCode  int
	headerWrote bool
	body        bytes.Buffer
}

func newRecorder(w http.ResponseWriter) *recorder {
	return &recorder{ResponseWriter: w, statusCode: http.StatusOK}
}

func (r *recorder) WriteHeader(code int) {
	if r.headerWrote {
		return
	}
	r.statusCode = code
	r.headerWrote = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if !r.headerWrote {
		r.WriteHeader(http.StatusOK)
	}
	r.body.Write(p)
	return r.ResponseWriter.Write(p)
}
