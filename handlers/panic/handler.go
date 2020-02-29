// Package panic defiens a panic handler that deals with panics occuring during
// the handling of a http request. It is in general route-agnostic.
package panic

import (
	"net/http"

	"context"

	"github.com/atdiar/xhttp"
)

// Handler allows for the registration of a panic handling function.
type Handler struct {
	Handle func(msg interface{}, ctx context.Context, w http.ResponseWriter, r *http.Request)
	next   xhttp.Handler
}

// NewHandler return an object used to take care of panics stemming from the
// request handling process accomodated by a downstrean chain of registered
// request handlers.
func NewHandler(handler func(msg interface{}, ctx context.Context, w http.ResponseWriter, r *http.Request)) Handler {
	return Handler{
		Handle: handler,
		next:   nil,
	}
}

// ServeHTTP handles the servicing of incoming http requests.
func (h Handler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	defer func() {
		if errmsg := recover(); errmsg != nil {
			h.Handle(errmsg, ctx, w, r)
		}
	}()
	if h.next != nil {
		h.next.ServeHTTP(ctx, w, r)
		return
	}
	panic("Panic Handler was ill-registered")
}

// Link enables the linking of a xhttp.Handler. The linked object holds the
// handling logic for the http request.
func (h Handler) Link(n xhttp.Handler) xhttp.HandlerLinker {
	h.next = n
	return h
}
