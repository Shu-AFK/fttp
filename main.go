package main

import (
	"fmt"
	httpRequest "httpServer/internal/request"
	httpResponse "httpServer/internal/response"
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

	for {
		conn, err := ln.Accept()
		if err != nil {
			panic(fmt.Errorf("failed to accept: %v", err))
		}

		req, err := httpRequest.HttpRequestParser(conn)
		if err != nil {
			fmt.Printf("failed to parse request: %v", err)
		}
		fmt.Printf("new connection from %v\n", conn.RemoteAddr())
		req.RemoteAddr = conn.RemoteAddr().String()
		fmt.Println(req)

		res := httpResponse.NewResponse(conn)
		res.Header().Set("Age", "999999")
		res.Header().Set("Date", "01/01/2024")
		res.Header().Add("Age", "1233")

		_, err = res.Write([]byte("Hello"))
		if err != nil {
			fmt.Printf("failed to write response: %v", err)
		}
	}
}
