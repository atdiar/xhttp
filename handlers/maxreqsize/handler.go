package maxreqsize

import (
	"context"
	"net/http"

	"github.com/atdiar/xhttp"
)

type Limiter struct {
	BodySize int64
	next     xhttp.Handler
}

func New(limit int) Limiter {
	return Limiter{int64(limit), nil}
}

func (l Limiter) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, l.BodySize)
	if l.next != nil {
		l.next.ServeHTTP(ctx, w, r)
	}
}

func (l Limiter) Link(h xhttp.Handler) xhttp.HandlerLinker {
	l.next = h
	return l
}
