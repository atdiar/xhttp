// Package redirect allows one to respond to a request with a "redirect to" URL.
package redirect

import (
	"net/http"

	"context"

	"github.com/atdiar/xhttp"
)

// Handler is the xhttp compatible version for the http.Redirect handler.
type Handler struct {
	URLstr string
	Code   int
	next   xhttp.Handler
}

// To returns a redirecting request handler.
func To(urlstr string, code int) Handler {
	return Handler{
		URLstr: urlstr,
		Code:   code,
		next:   nil,
	}
}

func (h Handler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.URLstr, h.Code)
	if h.next != nil {
	}
}

// Link registers a next request Handler to be called by ServeHTTP method.
// It returns the result of the linking.
func (h Handler) Link(nh xhttp.Handler) xhttp.HandlerLinker {
	h.next = nh
	return h
}
