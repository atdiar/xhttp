// Package panic defiens a panic handler that deals with panics occuring during
// the handling of a http request. It is in general route-agnostic.
package panic

import (
	"net/http"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

// Handler allows for the logging of the unhandled error resulting from the
// program panicking. It also allows for the registration of a specific
// panic handling function.
// The Log field is a function that can be setup in case the panic needs to be
// recorded in a log file locally or externally.
type Handler struct {
	Handle func(ctx execution.Context, w http.ResponseWriter, r *http.Request)
	Log    func(data ...interface{})
	next   xhttp.Handler
}

// NewHandler return an object used to take care of panics stemming from the
// request handling process accomodated by a downstrean chain of registered
// request handlers.
func NewHandler(handler xhttp.HandlerFunc) Handler {
	return Handler{
		Handle: handler.ServeHTTP,
		Log:    nil,
		next:   nil,
	}
}

// WithLogging allows to provide a logging function that will be used to
// record the error that explains why the request handling failed.
func (h Handler) WithLogging(f func(data ...interface{})) Handler {
	h.Log = f
	return h
}

// ServeHTTP handles the servicing of incoming http requests.
func (h Handler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			h.Handle(ctx, w, r)
			if h.Log != nil {
				h.Log(err)
			}
		}
	}()
	if h.next != nil {
		h.next.ServeHTTP(ctx, w, r)
		return
	}
	panic("Handler was ill-registered")
}

// Link enables the linking of a xhttp.Handler. The linked object holds the
// handling logic for the http request.
func (h Handler) Link(n xhttp.Handler) xhttp.HandlerLinker {
	h.next = n
	return h
}
