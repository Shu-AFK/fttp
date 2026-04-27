package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"httpServer/internal/proxy"
)

func main() {
	var certPath = flag.String("cert", "", "https server cert file")
	var keyPath = flag.String("key", "", "https server key file")
	var configFile = flag.String("config", "", "config file")

	flag.Parse()

	if *certPath == "" || *keyPath == "" || *configFile == "" {
		panic("Certification, key and config file args are required!")
	}

	cert, err := proxy.LoadCertificates(*certPath, *keyPath)
	if err != nil {
		panic(fmt.Errorf("failed to load certificates: %v", err))
	}

	p := proxy.New(*configFile)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Fprintf(os.Stderr, "\nReceived %s, shutting down...\n", sig)
		p.Stop()
	}()

	if err := p.Start(cert); err != nil {
		fmt.Printf("failed to start proxy: %v", err)
		os.Exit(1)
	}
}
