// Package redirect allows one to respond to a request with a "redirect to" URL.
package redirect

import (
	"net/http"
	"strings"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

// Handler is the xhttp compatible version for the http.Redirect handler.
type Handler struct {
	URLstr string
	Code   int
	next   xhttp.Handler
}

// NewHandler returns a redirecting request handler.
func NewHandler(urlstr string, code int) Handler {
	return Handler{
		URLstr: urlstr,
		Code:   code,
		next:   nil,
	}
}

func (h Handler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, h.URLstr, h.Code)
	if h.next != nil {
		h.next.ServeHTTP(ctx, w, r)
	}
}

// Link registers a next request Handler to be called by ServeHTTP method.
// It returns the result of the linking.
func (h Handler) Link(nh xhttp.Handler) xhttp.HandlerLinker {
	h.next = nh
	return h
}

// ToHTTPS returns a traffic redirecting handler that amkes sure that the web
// traffic between the client and the server uses a secure data transport
// protocol.
func ToHTTPS(r *http.Request) Handler {
	return NewHandler(strings.Replace(r.URL.String(), "http://", "https://", 1), 302)
}
