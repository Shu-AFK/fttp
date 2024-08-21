package handler

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	hpack "github.com/tatsuhiro-t/go-http2-hpack"
	http11 "httpServer/internal/request/http1.1"
	http2 "httpServer/internal/request/http2"
	http11Response "httpServer/internal/response/http1.1"
	http2Response "httpServer/internal/response/http2"
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
			fmt.Printf("[GET] Response writer failed with: %s\n", err)
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
		fmt.Printf("[GET BY ID] Response writer failed with: %s\n", err)
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

	if r.ProtoMajor == 2 {
		_, err := w.Write(nil)
		if err != nil {
			fmt.Printf("[METHOD NOT ALLOWED HANDLER] Response writer failed with: %s\n", err)
		}
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
	if ok {
		err := tlsConn.Handshake()
		if err != nil {
			fmt.Printf("[HANDLER] Handshake failed with: %s\n", err)
			return
		}
	}

	sendBadRequest := false
	moreRequests := false
	var req *http.Request
	var err error

	// TODO: Separate functions
	if !ok || tlsConn.ConnectionState().NegotiatedProtocol == "http/1.1" {
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
		dec := hpack.NewDecoder()

		// Validate settings frame
		err := http2Response.VerifyConnectionPreface(requestReader)
		if err != nil {
			fmt.Printf("failed to verify connection preface: %v\n", err)
			return
		}

		err = http2Response.SendSettingsFrame(tlsConn, requestReader)
		if err != nil {
			fmt.Printf("failed to send settings frame: %v\n", err)
			return
		}

		req, err := http2.Parser(requestReader, dec)
		if err != nil {
			fmt.Printf("failed to parse requests: %v\n", err)
			return
		}

		counter := 0
		for streamId, _ := range http2.Channels {
			req[counter].RemoteAddr = conn.RemoteAddr().String()

			responseWriter := http2Response.NewResponse(conn, streamId)
			r.ServeHTTP(responseWriter, req[counter])
			err := http2Response.SendFrame(conn, http2.DATA_FRAME_TYPE, http2.END_STREAM, streamId, nil)
			if err != nil {
				fmt.Printf("failed to send frame: %v\n", err)
			}

			counter++
		}
	}

	fmt.Printf("handled connection from %v\n", conn.RemoteAddr())
}
