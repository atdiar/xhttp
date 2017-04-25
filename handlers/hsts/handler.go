// Package hsts is used to enforce a Strict Transport Security.
package hsts

import (
	"net/http"
	"strconv"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

// Handler is an object that enforces the use of Strict Transport Security.
type Handler struct {
	m                 int
	includeSubDomains bool
	next              xhttp.Handler
}

// New is a handler that enforces the use of Strict Transport Security.
func New(maxage int, withsubdomains bool) Handler {
	return Handler{
		m:                 maxage,
		includeSubDomains: withsubdomains,
		next:              nil,
	}
}

func (h Handler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	if h.includeSubDomains {
		w.Header().Add("Strict-Transport-Security", "max-age="+strconv.Itoa(h.m)+"; includeSubDomains")
	} else {
		w.Header().Add("Strict-Transport-Security", "max-age="+strconv.Itoa(h.m))
	}
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
