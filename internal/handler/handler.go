package handler

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	http11 "httpServer/internal/request/http1.1"
	"httpServer/internal/request/http2"
	http11Response "httpServer/internal/response/http1.1"
	"io"
	"net"
	"net/http"
)

var notes = make(map[string]string)

func PutNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	bodyReader := r.Body

	bodyBuffer, err := io.ReadAll(bodyReader)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	notes[id] = string(bodyBuffer)
	w.WriteHeader(http.StatusCreated)
}

func DeleteNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	delete(notes, id)

	w.WriteHeader(http.StatusNoContent)
}

func GetNotes(w http.ResponseWriter, r *http.Request) {
	if len(notes) != 0 {
		var notesContent string

		for k, v := range notes {
			notesContent += fmt.Sprintf("%s -> %s\n\n", k, v)
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte(notesContent))
		if err != nil {
			fmt.Printf("[GET] Response writer failed with: %s", err)
		}

	} else {
		w.WriteHeader(http.StatusNoContent)
	}
}

func GetNoteById(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	note, ok := notes[id]

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(note))
	if err != nil {
		fmt.Printf("[GET BY ID] Response writer failed with: %s", err)
		return
	}
}

func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	_, err := w.Write([]byte("Not Found"))
	if err != nil {
		fmt.Printf("[NOT FOUND HANDLER] Response writer failed with: %s\n", err)
	}
}

func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusMethodNotAllowed)
	_, err := w.Write([]byte("Method Not Allowed"))
	if err != nil {
		fmt.Printf("[METHOD NOT ALLOWED HANDLER] Response writer failed with: %s\n", err)
	}
}

func HandleAccept(conn net.Conn, r chi.Router) {
	defer func(conn net.Conn) {
		err := conn.Close()
		if err != nil {
			fmt.Println("Error closing connection")
		}
	}(conn)

	fmt.Printf("new connection from %v\n", conn.RemoteAddr())

	tlsConn, ok := conn.(*tls.Conn)

	sendBadRequest := false
	moreRequests := false
	var req *http.Request
	var err error

	// TODO: Separate functions
	if tlsConn.ConnectionState().NegotiatedProtocol == "http/1.1" || !ok {
		requestReader := bufio.NewReader(conn)
		for {
			req, err, moreRequests = http11.Parser(requestReader)

			if err != nil {
				if errors.Is(err, http11.ChunkEncodingError) {
					sendBadRequest = true
				} else {
					fmt.Printf("failed to parse request: %v\n", err)
					return
				}
			}

			req.RemoteAddr = conn.RemoteAddr().String()

			responseWriter := http11Response.NewResponse(conn)
			if sendBadRequest {
				responseWriter.WriteHeader(http.StatusBadRequest)
				fmt.Printf("[BAD REQUEST] Request parser failed due to wrong placement of chunked encoding\n")
			} else {
				r.ServeHTTP(responseWriter, req)
			}

			if !moreRequests {
				break
			}
		}
	} else if tlsConn.ConnectionState().NegotiatedProtocol == "h2" {
		requestReader := bufio.NewReader(tlsConn)

		// TODO: finish
		req, err := http2.Parser(requestReader)
		if err != nil {

		}
		println(req)
		panic(errors.New("not implemented yet"))
	}

	fmt.Printf("handled connection from %v\n", conn.RemoteAddr())
}
