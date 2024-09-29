package cache_structs

import (
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
	TimeCached time.Time
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

type CacheStruct struct {
	Cache map[Request]Response
	Mutex sync.RWMutex
}

type Channels struct {
	Requests   chan Request
	Responses  chan Response
	Found      chan bool
	AddToCache chan AddToCacheStruct
}
