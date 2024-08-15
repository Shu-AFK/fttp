package main

import (
	"fmt"
	httpRequest "httpServer/internal/request"
	httpResponse "httpServer/internal/response"
	"net"
	"time"
)

func handleAccept(conn net.Conn) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Println("Error closing connection")
		}
	}(conn)

	req, err := httpRequest.Parser(conn)
	if err != nil {
		fmt.Printf("failed to parse request: %v\n", err)
	}
	fmt.Printf("new connection from %v\n", conn.RemoteAddr())
	req.RemoteAddr = conn.RemoteAddr().String()
	fmt.Println(req)

	res := httpResponse.NewResponse(conn)
	res.Header().Set("Age", "999999")
	res.Header().Set("Date", "01/01/2024")
	res.Header().Add("Age", "1233")

	message := "Hello"

	w, err := res.Write([]byte(message))
	if err != nil {
		fmt.Printf("failed to write response: %v\n", err)
	}
	if w != len([]byte(message)) {
		fmt.Printf("failed to write: expected: %d wrote: %d", len([]byte(message)), w)
	}
	time.Sleep(10 * time.Millisecond)
}

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

	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(fmt.Errorf("failed to accept: %v", err))
		}

		go handleAccept(conn)
	}
}
