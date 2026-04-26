package proxy

import "net/url"

type ProxyRoute struct {
	Path       string
	Host       *url.URL
	TargetPath string
}
