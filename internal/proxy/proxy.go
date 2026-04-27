package proxy

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"httpServer/internal/cache"
	"httpServer/internal/logging"
)

type Proxy struct {
	Port          uint16
	Routes        []ProxyRoute
	AddedHeaders  http.Header
	CachingActive bool
	CachingTTL    time.Duration
	Blacklist     []net.IP
	Logger        logging.Logger

	listener net.Listener
	cache    *cache.Cache
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func New(configPath string) *Proxy {
	conf, err := LoadConfig(configPath)
	if err != nil {
		panic(err)
	}

	var routes []ProxyRoute
	for _, route := range conf.Server.Routes {
		parsedHost, err := url.Parse(route.Host)
		if err != nil {
			log.Fatalf("Failed to parse host URL %s: %v", route.Host, err)
		}

		hostname := parsedHost.Hostname()
		IPs, err := net.LookupIP(hostname)
		if err != nil {
			log.Fatalf("Failed to resolve IP for host %s: %v", hostname, err)
		}
		if len(IPs) == 0 {
			log.Fatalf("No IP addresses found for host %s", hostname)
		}

		ip := IPs[0]
		resolvedHost := ip.String()
		if ip.To4() == nil {
			resolvedHost = fmt.Sprintf("[%s]", resolvedHost)
		}
		resolvedURL := fmt.Sprintf("%s://%s", parsedHost.Scheme, resolvedHost)
		if parsedHost.Port() != "" {
			resolvedURL = fmt.Sprintf("%s:%s", resolvedURL, parsedHost.Port())
		}

		parsedURL, err := url.Parse(resolvedURL)
		if err != nil {
			log.Fatalf("Failed to parse resolved host URL %s: %v", resolvedURL, err)
		}

		routes = append(routes, ProxyRoute{
			Path:       route.Path,
			Host:       parsedURL,
			TargetPath: route.TargetPath,
		})
	}

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
		AddedHeaders:  conf.AddHeader,
		stop:          make(chan struct{}),
	}
}

// CacheStats returns the current cache hit/miss counters. Returns zeros if
// caching is disabled.
func (p *Proxy) CacheStats() (hits, misses int64) {
	if p.cache == nil {
		return 0, 0
	}
	return p.cache.Hits(), p.cache.Misses()
}

// Stop signals Start to exit, closes the listener, waits for in-flight
// connections to finish, and stops the cache sweeper. Safe to call multiple
// times; safe to call before Start has fully booted.
func (p *Proxy) Stop() {
	p.stopOnce.Do(func() {
		close(p.stop)
		if p.listener != nil {
			_ = p.listener.Close()
		}
		if p.cache != nil {
			p.cache.Stop()
		}
	})
}

func (p *Proxy) Log(level logging.LogLevel, message string, args ...interface{}) {
	p.Logger.Log(level, message, args...)
}

func (p *Proxy) GetCachingTTL() time.Duration {
	return p.CachingTTL
}

func (p *Proxy) closeIfBlacklisted(conn net.Conn) bool {
	remoteIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		p.Log(logging.LogLevelError, "Failed to parse remote address: %v", err)
		_ = conn.Close()
		return true
	}

	for _, blacklistedIP := range p.Blacklist {
		if net.ParseIP(remoteIP).Equal(blacklistedIP) {
			p.Log(logging.LogLevelDebug, "Detected blacklisted IP: %s", remoteIP)
			if err := conn.Close(); err != nil {
				p.Log(logging.LogLevelError, "Failed to close connection from blacklisted IP: %v", err)
			}
			return true
		}
	}

	p.Log(logging.LogLevelDebug, "IP address %s is not blacklisted", remoteIP)
	return false
}

func (p *Proxy) Start(cert []tls.Certificate) error {
	p.Log(logging.LogLevelInfo, "Starting proxy server on port %d", p.Port)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p.Port))
	if err != nil {
		p.Log(logging.LogLevelError, "Failed to listen on port %d: %v", p.Port, err)
		return err
	}

	tlsConfig := &tls.Config{
		NextProtos:   []string{"h2", "http/1.1"},
		Certificates: cert,
	}
	tlsListener := tls.NewListener(ln, tlsConfig)
	p.listener = tlsListener

	p.Log(logging.LogLevelDebug, "Setting up router with provided routes")

	if p.CachingActive {
		p.cache = cache.New(p)
	}

	r := chi.NewRouter()
	r.NotFound(p.NotFoundHandler)
	r.MethodNotAllowed(p.MethodNotAllowedHandler)

	handler := cache.Middleware(p.cache, p.ReverseProxyHandler)
	for _, route := range p.Routes {
		pattern := strings.TrimRight(route.Path, "/")
		r.HandleFunc(pattern, handler)
		r.HandleFunc(pattern+"/*", handler)
		p.Log(logging.LogLevelDebug, "Added route: %s", route.Path)
	}

	p.Log(logging.LogLevelInfo, "Listening on https://%s", ln.Addr().String())

	for {
		conn, err := tlsListener.Accept()
		if err != nil {
			select {
			case <-p.stop:
				p.Log(logging.LogLevelInfo, "Shutdown requested, waiting for in-flight connections")
				p.wg.Wait()
				p.Log(logging.LogLevelInfo, "Proxy stopped cleanly")
				return nil
			default:
				p.Log(logging.LogLevelError, "Failed to accept connection: %v", err)
				continue
			}
		}

		p.Log(logging.LogLevelInfo, "Accepted new connection from %v", conn.RemoteAddr())

		if p.closeIfBlacklisted(conn) {
			p.Log(logging.LogLevelWarn, "Closed connection from blacklisted IP: %v", conn.RemoteAddr())
			continue
		}

		p.wg.Add(1)
		go func(conn net.Conn) {
			defer p.wg.Done()
			p.Log(logging.LogLevelDebug, "Handling connection from %v", conn.RemoteAddr())
			p.handleAccept(conn, r)
		}(conn)
	}
}
