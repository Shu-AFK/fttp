package reverseproxy

import (
	"crypto/tls"
	"fmt"
	"httpServer/internal/logging"
	"net"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"httpServer/internal/handler"
	"httpServer/internal/reverseproxy/structs"
)

type Proxy struct {
	Port          uint16
	Routes        []structs.ProxyRoute
	CachingActive bool
	CachingTTL    time.Duration
	Blacklist     []net.IP
	Logger        logging.Logger
}

func NewReverseProxy(configPath string) *Proxy {
	conf, err := LoadConfig(configPath)
	if err != nil {
		panic(err)
	}

	if err := conf.Validate(); err != nil {
		panic(err)
	}

	// Parse routes
	var routes []structs.ProxyRoute
	for _, route := range conf.Server.Routes {
		routes = append(routes, structs.ProxyRoute{
			Path:   route.Path,
			Target: net.ParseIP(route.Target),
		})
	}

	// Parse blacklist IPs
	var blacklist []net.IP
	for _, ipStr := range conf.Blacklist {
		blacklist = append(blacklist, net.ParseIP(ipStr))
	}

	logger, err := logging.NewDefaultLogger(logging.LogLevel(strings.ToUpper(conf.Logger.Level)), conf.Logger.File)
	if err != nil {
		panic(err)
	}

	return &Proxy{
		Port:          uint16(conf.Server.Port),
		Routes:        routes,
		CachingActive: conf.Caching.Enabled,
		CachingTTL:    time.Duration(conf.Caching.TTL) * time.Second,
		Blacklist:     blacklist,
		Logger:        logger,
	}
}

func (proxy *Proxy) CloseIfBlacklisted(conn net.Conn) bool {
	return proxy.closeIfBlacklisted(conn)
}

func (proxy *Proxy) GetPort() uint16 {
	return proxy.Port
}

func (proxy *Proxy) GetRoutes() []structs.ProxyRoute {
	return proxy.Routes
}

func (proxy *Proxy) IsCachingActive() bool {
	return proxy.CachingActive
}

func (proxy *Proxy) GetBlacklist() []net.IP {
	return proxy.Blacklist
}

func (proxy *Proxy) Log(level logging.LogLevel, message string, args ...interface{}) {
	proxy.Logger.Log(level, message, args...)
}

func (proxy *Proxy) closeIfBlacklisted(conn net.Conn) bool {
	remoteIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		proxy.Log(logging.LogLevelError, "Failed to parse remote address: %v", err)
		_ = conn.Close() // Close the connection due to a malformed address
		return true
	}

	for _, blacklistedIP := range proxy.Blacklist {
		if net.ParseIP(remoteIP).Equal(blacklistedIP) {
			proxy.Log(logging.LogLevelDebug, "Detected blacklisted IP: %s", remoteIP)
			if err := conn.Close(); err != nil {
				proxy.Log(logging.LogLevelError, "Failed to close connection from blacklisted IP: %v", err)
			}
			return true // Connection was closed
		}
	}

	proxy.Log(logging.LogLevelDebug, "IP address %s is not blacklisted", remoteIP)
	return false // Connection was not closed
}

func (proxy *Proxy) Start(cert []tls.Certificate) error {
	proxy.Log(logging.LogLevelInfo, "Starting proxy server on port %d", proxy.Port)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", proxy.Port))
	if err != nil {
		proxy.Log(logging.LogLevelError, "Failed to listen on port %d: %v", proxy.Port, err)
		return err
	}

	tlsConfig := &tls.Config{
		NextProtos:   []string{"h2", "http/1.1"},
		Certificates: cert,
	}
	tlsListener := tls.NewListener(ln, tlsConfig)

	defer func() {
		if cerr := ln.Close(); cerr != nil {
			proxy.Log(logging.LogLevelError, "Failed to close listener: %v", cerr)
		}
	}()

	proxy.Log(logging.LogLevelDebug, "Setting up router with provided routes")

	r := chi.NewRouter()
	r.NotFound(handler.NotFoundHandler)
	r.MethodNotAllowed(handler.MethodNotAllowedHandler)

	for _, route := range proxy.Routes {
		r.HandleFunc(route.Path, handler.ReverseProxyHandler)
		proxy.Log(logging.LogLevelDebug, "Added route: %s", route.Path)
	}

	proxy.Log(logging.LogLevelInfo, "Listening on https://%s", ln.Addr().String())

	for {
		conn, err := tlsListener.Accept()
		if err != nil {
			proxy.Log(logging.LogLevelError, "Failed to accept connection: %v", err)
			continue
		}

		proxy.Log(logging.LogLevelInfo, "Accepted new connection from %v", conn.RemoteAddr())

		if proxy.closeIfBlacklisted(conn) {
			proxy.Log(logging.LogLevelWarn, "Closed connection from blacklisted IP: %v", conn.RemoteAddr())
			continue
		}

		go func(conn net.Conn) {
			proxy.Log(logging.LogLevelDebug, "Handling connection from %v", conn.RemoteAddr())
			handler.HandleAccept(conn, proxy, r)
		}(conn)
	}
}
