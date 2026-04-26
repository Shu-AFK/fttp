package cache

import (
	"net/http"
	"time"
)

type entry struct {
	statusCode int
	header     http.Header
	body       []byte
	expires    time.Time
}
