package cache

// TODO: Add function to turn Request and Response back into http. version

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"httpServer/internal/logging"
)

// Host is the narrow interface the cache needs from its caller. Defined here
// (rather than in the proxy package) so cache stays import-cycle free.
type Host interface {
	Log(level logging.LogLevel, format string, args ...interface{})
	GetCachingTTL() time.Duration
}

var cache *CacheStruct
var ttl time.Duration
var host Host

func InitCache(h Host, channels Channels) {
	cache = new(CacheStruct)
	cache.Cache = make(map[Request]Response)
	cache.Mutex = sync.RWMutex{}

	ttl = h.GetCachingTTL()
	host = h

	host.Log(logging.LogLevelDebug, "Starting Cache")
	go startCaching(channels.Requests, channels.Responses, channels.Found)
	go cleanupCache()
	go addToCache(channels.AddToCache)
}

// TODO: Need a more sophisticated approach
func cleanupCache() {
	for {
		time.Sleep(ttl / 2)
		cache.Mutex.RLock()
		for req := range cache.Cache {
			if time.Since(req.TimeCached) > ttl {
				delete(cache.Cache, req)
			}
		}
		cache.Mutex.RUnlock()
	}
}

func addToCache(in chan AddToCacheStruct) {
	for add := range in {
		outRequest := turnReqToCacheRequest(add.Request)
		outResponse := turnRespToCacheResponse(add.Response)
		cache.Mutex.RLock()
		cache.Cache[outRequest] = outResponse
		cache.Mutex.RUnlock()
	}
}

func startCaching(requests chan Request, responses chan Response, found chan bool) {
	for request := range requests {
		cache.Mutex.RLock()
		val, ok := cache.Cache[request]
		cache.Mutex.RUnlock()
		if ok {
			host.Log(logging.LogLevelDebug, fmt.Sprintf("Cache hit for request: %s", request.URL.String()))
			found <- true
			responses <- val
		} else {
			host.Log(logging.LogLevelDebug, fmt.Sprintf("Cache miss for request: %s", request.URL.String()))
			found <- false
		}
	}
}

func turnReqToCacheRequest(request http.Request) Request {
	return Request{
		Method:     request.Method,
		URL:        request.URL,
		RequestURI: request.RequestURI,
		TimeCached: time.Now(),
	}
}

func turnRespToCacheResponse(response http.Response) Response {
	headerCopy := make(http.Header)
	for k, v := range response.Header {
		headerCopy[k] = v
	}

	return Response{
		StatusCode:    response.StatusCode,
		Body:          response.Body,
		ContentLength: response.ContentLength,
		Header:        headerCopy,
	}
}
