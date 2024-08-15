// https://github.com/go-chi/chi/blob/master/_examples/custom-handler/main.go

package handler

import "net/http"

type Handler func(http.ResponseWriter, *http.Request) error

func (h Handler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if err := h(resp, req); err != nil {
		resp.WriteHeader(http.StatusInternalServerError)
		_, err = resp.Write([]byte("error"))
	}
}
