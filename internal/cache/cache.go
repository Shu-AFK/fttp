package cache

import (
	"fmt"
	"httpServer/internal/logging"
	"httpServer/internal/reverseproxy/structs"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Request struct {
	Method     string
	URL        *url.URL
	RequestURI string
	timeCached time.Time
}

type Response struct {
	StatusCode    int
	Body          io.ReadCloser
	ContentLength int64
	Header        http.Header
	timeCached    time.Time
}

type cacheStruct struct {
	Cache map[Request]Response
	Mutex sync.RWMutex
}

type Channels struct {
	Requests  chan Request
	Responses chan Response
	Found     chan bool
}

var cache *cacheStruct
var ttl time.Duration
var proxy structs.ProxyHandler

func InitCache(reverseProxy structs.ProxyHandler, channels Channels) {
	cache = new(cacheStruct)
	cache.Cache = make(map[Request]Response)
	cache.Mutex = sync.RWMutex{}

	ttl = reverseProxy.GetCachingTTL()
	proxy = reverseProxy

	proxy.Log(logging.LogLevelDebug, "Starting Cache")
	go startCaching(channels.Requests, channels.Responses, channels.Found)
	go cleanupCache()
}

// TODO: Need a more sophisticated approach
func cleanupCache() {
	for {
		time.Sleep(ttl / 2)
		cache.Mutex.Lock()
		for req, resp := range cache.Cache {
			if time.Since(resp.timeCached) > ttl {
				delete(cache.Cache, req)
			}
		}
		cache.Mutex.Unlock()
	}
}

func startCaching(requests chan Request, responses chan Response, found chan bool) {
	for {
		select {
		case request := <-requests:
			cache.Mutex.Lock()
			val, ok := cache.Cache[request]
			cache.Mutex.Unlock()
			if ok {
				proxy.Log(logging.LogLevelDebug, fmt.Sprintf("Cache hit for request: %s", request.URL.String()))
				found <- true
				responses <- val
			}

			if !ok {
				proxy.Log(logging.LogLevelDebug, fmt.Sprintf("Cache miss for request: %s", request.URL.String()))
				found <- false
			}
		}
	}
}
