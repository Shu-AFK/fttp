package proxy

import (
	"crypto/tls"
	"net/http"
	"net/url"
)

type ProxyRoute struct {
	Path       string
	Host       *url.URL
	TargetPath string
	// Client is the http.Client used to forward to this route's upstream.
	// Constructed once in New() so each route reuses connections and
	// applies its own TLS verification policy.
	Client *http.Client
}

func newRouteClient(insecureSkipVerify bool) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecureSkipVerify,
			},
		},
	}
}
