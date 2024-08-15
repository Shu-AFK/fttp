package main

import (
	"fmt"
	"github.com/go-chi/chi/v5"
	"httpServer/internal/handler"
	"net"
)

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		panic(fmt.Errorf("failed to listen: %v", err))
	}
	defer func(ln net.Listener) {
		err := ln.Close()
		if err != nil {
			fmt.Printf("failed to close listener: %v", err)
		}
	}(ln)

	r := chi.NewRouter()
	r.Put("/v1/notes/{id}", handler.PutNote)
	r.Delete("/v1/notes/{id}", handler.DeleteNote)
	r.Get("/v1/notes", handler.GetNotes)
	r.Get("/v1/notes/{id}", handler.GetNoteById)

	r.NotFound(handler.NotFoundHandler)
	r.MethodNotAllowed(handler.MethodNotAllowedHandler)

	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(fmt.Errorf("failed to accept: %v", err))
		}

		go handler.HandleAccept(conn, r)
	}
}
