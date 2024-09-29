package cache

// TODO: Add function to turn Request and Response back into http. version

import (
	"fmt"
	cache_structs "httpServer/internal/cache/structs"
	"httpServer/internal/logging"
	"httpServer/internal/reverseproxy/structs"
	"net/http"
	"sync"
	"time"
)

var cache *cache_structs.CacheStruct
var ttl time.Duration
var proxy structs.ProxyHandler

func InitCache(reverseProxy structs.ProxyHandler, channels cache_structs.Channels) {
	cache = new(cache_structs.CacheStruct)
	cache.Cache = make(map[cache_structs.Request]cache_structs.Response)
	cache.Mutex = sync.RWMutex{}

	ttl = reverseProxy.GetCachingTTL()
	proxy = reverseProxy

	proxy.Log(logging.LogLevelDebug, "Starting Cache")
	go startCaching(channels.Requests, channels.Responses, channels.Found)
	go cleanupCache()
	go addToCache(channels.AddToCache)
}

// TODO: Need a more sophisticated approach
func cleanupCache() {
	for {
		time.Sleep(ttl / 2)
		cache.Mutex.RLock()
		for req, _ := range cache.Cache {
			if time.Since(req.TimeCached) > ttl {
				delete(cache.Cache, req)
			}
		}
		cache.Mutex.RUnlock()
	}
}

func addToCache(AddToCache chan cache_structs.AddToCacheStruct) {
	for {
		select {
		case add := <-AddToCache:
			outRequest := turnReqToCacheRequest(add.Request)
			outResponse := turnRespToCacheResponse(add.Response)
			cache.Mutex.RLock()
			cache.Cache[outRequest] = outResponse
			cache.Mutex.RUnlock()
		}
	}
}

func startCaching(requests chan cache_structs.Request, responses chan cache_structs.Response, found chan bool) {
	for {
		select {
		case request := <-requests:
			cache.Mutex.RLock()
			val, ok := cache.Cache[request]
			cache.Mutex.RUnlock()
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

func turnReqToCacheRequest(request http.Request) cache_structs.Request {
	return cache_structs.Request{
		Method:     request.Method,
		URL:        request.URL,
		RequestURI: request.RequestURI,
		TimeCached: time.Now(),
	}
}

func turnRespToCacheResponse(response http.Response) cache_structs.Response {
	headerCopy := make(http.Header)
	for k, v := range response.Header {
		headerCopy[k] = v
	}

	return cache_structs.Response{
		StatusCode:    response.StatusCode,
		Body:          response.Body,
		ContentLength: response.ContentLength,
		Header:        headerCopy,
	}
}
