package main

import (
	"fmt"
	"net/http"
)

func loggingHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("Received request:")
		fmt.Printf("Method: %s\n", r.Method)
		fmt.Printf("URL: %s\n", r.URL.Path)
		fmt.Printf("Remote address: %s\n", r.RemoteAddr)
		fmt.Println("Headers:")

		for key, values := range r.Header {
			for _, value := range values {
				fmt.Printf("  %s: %s\n", key, value)
			}
		}

		next(w, r)
	}
}

func main() {
	http.HandleFunc("/api/v1", loggingHandler(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent) // changed this line
	}))

	http.ListenAndServe(":3000", nil)
}
