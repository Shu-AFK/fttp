package cache

import "time"

type Request struct {
}

type Response struct {
}

var cache map[Request]Response
var ttl time.Duration

func InitCache(duration time.Duration) {
	cache = make(map[Request]Response)
	ttl = duration
}
