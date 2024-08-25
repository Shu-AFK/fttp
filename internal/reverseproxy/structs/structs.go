package structs

import (
	"httpServer/internal/logging"
	"net"
	"net/url"
)

type ProxyRoute struct {
	Path   string
	Target *url.URL
}

type ProxyHandler interface {
	Log(level logging.LogLevel, message string, args ...interface{})
	CloseIfBlacklisted(conn net.Conn) bool
	GetPort() uint16
	GetRoutes() []ProxyRoute
	IsCachingActive() bool
	GetBlacklist() []net.IP
}
