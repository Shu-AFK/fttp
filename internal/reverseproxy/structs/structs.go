package structs

import (
	"httpServer/internal/logging"
	"net"
)

type ProxyRoute struct {
	Path   string
	Target net.IP
}

type ProxyHandler interface {
	Log(level logging.LogLevel, message string, args ...interface{})
	CloseIfBlacklisted(conn net.Conn) bool
	GetPort() uint16
	GetRoutes() []ProxyRoute
	IsCachingActive() bool
	GetBlacklist() []net.IP
}
