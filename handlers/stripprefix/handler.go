package stripprefix

import (
	"net/http"
	"strings"

	"context"

	"github.com/atdiar/xhttp"
)

// Handler returns a xhttp.Handler which sole purpose is to remove a given
// prefix from the http request URL path.
// If the prefix does not exist, a 404 HTTP error is sent.
type Handler struct {
	prefix string
	next   xhttp.Handler
}

// NewHandler returns a request Handler whose task is simply to mutate the
// request object by stripping it from a given prefix.
func NewHandler(prefix string) Handler {
	return Handler{
		prefix: prefix,
		next:   nil,
	}
}

func (h Handler) ServeHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	if h.prefix == "" {
		if h.next != nil {
			h.next.ServeHTTP(ctx, w, r)
			return
		}
	}
	if p := strings.TrimPrefix(r.URL.Path, h.prefix); len(p) < len(r.URL.Path) {
		r.URL.Path = p
		if h.next != nil {
			h.next.ServeHTTP(ctx, w, r)
			return
		}
	} else {
		http.NotFound(w, r)
		return
	}
}

// Link registers a next request Handler to be called by ServeHTTP method.
// It returns the result of the linking.
func (h Handler) Link(nh xhttp.Handler) xhttp.HandlerLinker {
	h.next = nh
	return h
}
