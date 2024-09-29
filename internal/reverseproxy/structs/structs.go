package structs

import (
	cache_structs "httpServer/internal/cache/structs"
	"httpServer/internal/logging"
	"net"
	"net/http"
	"net/url"
	"time"
)

type ProxyRoute struct {
	Path       string
	Host       *url.URL
	TargetPath string
}

type ProxyHandler interface {
	Log(level logging.LogLevel, message string, args ...interface{})
	CloseIfBlacklisted(conn net.Conn) bool
	GetPort() uint16
	GetRoutes() []ProxyRoute
	IsCachingActive() bool
	GetBlacklist() []net.IP
	GetAddedHeaders() http.Header
	GetCachingTTL() time.Duration
	GetCachingChannels() cache_structs.Channels
}
