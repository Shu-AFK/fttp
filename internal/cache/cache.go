package cache

// TODO: Add function to turn Request and Response back into http. version

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
}

type AddToCacheStruct struct {
	Request  http.Request
	Response http.Response
}

type cacheStruct struct {
	Cache map[Request]Response
	Mutex sync.RWMutex
}

type Channels struct {
	Requests   chan Request
	Responses  chan Response
	Found      chan bool
	AddToCache chan AddToCacheStruct
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
	go addToCache(channels.AddToCache)
}

// TODO: Need a more sophisticated approach
func cleanupCache() {
	for {
		time.Sleep(ttl / 2)
		cache.Mutex.RLock()
		for req, _ := range cache.Cache {
			if time.Since(req.timeCached) > ttl {
				delete(cache.Cache, req)
			}
		}
		cache.Mutex.RUnlock()
	}
}

func addToCache(AddToCache chan AddToCacheStruct) {
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

func startCaching(requests chan Request, responses chan Response, found chan bool) {
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

func turnReqToCacheRequest(request http.Request) Request {
	return Request{
		Method:     request.Method,
		URL:        request.URL,
		RequestURI: request.RequestURI,
		timeCached: time.Now(),
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
