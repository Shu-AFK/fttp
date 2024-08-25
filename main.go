package main

import (
	"flag"
	"fmt"
	"httpServer/internal/helper"
	"httpServer/internal/reverseproxy"
)

func main() {
	var certPath = flag.String("cert", "", "https server cert file")
	var keyPath = flag.String("key", "", "https server key file")
	var configFile = flag.String("config", "", "config file")

	flag.Parse()

	if *certPath == "" || *keyPath == "" || *configFile == "" {
		panic("Certification, key and config file args are required!")
	}

	cert, err := helper.LoadCertificates(*certPath, *keyPath)
	if err != nil {
		panic(fmt.Errorf("failed to load certificates: %v", err))
	}

	proxy := reverseproxy.NewReverseProxy(*configFile)
	err = proxy.Start(cert)
	if err != nil {
		fmt.Printf("failed to start proxy: %v", err)
		return
	}
}
