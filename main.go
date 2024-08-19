package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"github.com/go-chi/chi/v5"
	"httpServer/internal/handler"
	helper "httpServer/internal/helper"
	"net"
)

func main() {
	var certPath = flag.String("cert", "", "https server cert file")
	var keyPath = flag.String("key", "", "https server key file")

	flag.Parse()

	if *certPath == "" || *keyPath == "" {
		panic("Certification and key args are required!")
	}

	cert, err := helper.LoadCertificates(*certPath, *keyPath)
	if err != nil {
		panic(fmt.Errorf("failed to load certificates: %v", err))
	}

	ln, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(fmt.Errorf("failed to listen: %v", err))
	}

	tlsConfig := &tls.Config{
		NextProtos:   []string{"h2", "http/1.1"},
		Certificates: cert,
	}
	tlsListener := tls.NewListener(ln, tlsConfig)

	defer func(ln net.Listener) {
		err := ln.Close()
		if err != nil {
			fmt.Printf("failed to close listener: %v", err)
		}
	}(tlsListener)

	r := chi.NewRouter()
	r.Put("/v1/notes/{id}", handler.PutNote)
	r.Delete("/v1/notes/{id}", handler.DeleteNote)
	r.Get("/v1/notes", handler.GetNotes)
	r.Get("/v1/notes/{id}", handler.GetNoteById)

	r.NotFound(handler.NotFoundHandler)
	r.MethodNotAllowed(handler.MethodNotAllowedHandler)

	fmt.Printf("listening on https://%s\n", ln.Addr().String())

	for {
		conn, err := tlsListener.Accept()
		if err != nil {
			panic(fmt.Errorf("failed to accept: %v", err))
		}

		go handler.HandleAccept(conn, r)
	}
}
