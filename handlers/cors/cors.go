// Package cors defines a request handler used to enforce the Cross Origin
// Resource Sharing policy of a server.
package cors

import (
	"net/http"
	"net/textproto"
	"strings"
	"time"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

// Access control reference: https://www.w3.org/TR/2008/WD-access-control-20080912/

/*
Rationale
=========

A Cross Origin http request for a given resource is made when a user agent is
used to retrieve resources from a  given domain which themselves depend on
resources from another domain (such as an image stored on a CDN for instance).

The current package can be used to specify the conditions under which we allow a
resource at a given endpoint to be accessed.

The default being `same-origin` policy (same domain, same protocol, same port,
same host), it can be relaxed by specifying some headers that will be captured
and relayed to the client in response to a preflight request.
Presence of these headers determines whether a resource is accessible.


*/

var (
	// SimpleRequestMethods is the set of methods for which CORS is allowed
	// without preflight.
	SimpleRequestMethods = newSet().Add("GET", "HEAD", "POST")

	// SimpleRequestHeaders is the set of headers for which CORS is allowed
	// without preflight.
	SimpleRequestHeaders = newSet().Add("Accept", "Accept-Language", "Content-Language", "Content-Type")

	// SimpleRequestContentTypes is the set of headers for which CORS is allowed
	// without preflight.
	SimpleRequestContentTypes = newSet().Add("application/x-www-form-urlencoded", "multipart/form-data", "text/plain")
)

// Handler is used to specify and enforce a Cross Origin Resource Sharing policy
// on incoming http requests.
// CORS controls the access to resources available on the server by defining
// constraints (request origin, http methods allowed, headers allowed, etc.)
type Handler struct {
	AllowedOrigins      set
	AllowedHeaders      set
	AllowedContentTypes set
	ExposeHeaders       set
	AllowedMethods      set
	AllowCredentials    bool

	// determines the validity in cache of the preflight result
	MaxAge time.Duration

	preflightCache session.Cache

	next xhttp.Handler
}

// NewHandler creates a new CORS policy enforcing request handler.
func NewHandler() Handler {
	h := Handler{}
	h.AllowedMethods = newSet().Add("GET", "HEAD", "POST")
	h.AllowedHeaders = newSet().Add("Accept", "Accept-Language", "Content-Language", "Content-Type")
	h.AllowedContentTypes = newSet().Add("application/x-www-form-urlencoded", "multipart/form-data", "text/plain")
	h.MaxAge = 10 * time.Minute
	return h
}

// WithPreflight will allow the handling of preflighted requests via the
// OPTIONS http method.
// Preflight result will be cached.
func (h Handler) WithPreflight(c session.Cache) Handler {
	h.AllowedMethods = h.AllowedMethods.Add("OPTIONS")
	h.preflightCache = c
	return h
}

// WithCredentials will allow the emmission of cookies, authorization headers,
// TLS client certificates with the http requests by the client.
func (h Handler) WithCredentials() Handler {
	h.AllowCredentials = true
	return h
}

func (h Handler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	if !originIsPresent(r) {
		http.Error(w, "Bad Request", 400)
		return
	}
	// Test whether the request is a simple request
	if methodIsAllowed(r, SimpleRequestMethods) {
		if headersAreAllowed(r, SimpleRequestHeaders) {
			if contentTypeIsAllowed(r, SimpleRequestContentTypes) {
				setAllowOrigin(w, r)
				setAllowCredentials(w, false)
				setAllowMethods(w, SimpleRequestMethods)

			}
		}
	}
}

func setAllowOrigin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", textproto.MIMEHeader(r.Header).Get("Origin"))
}

func setAllowMethods(w http.ResponseWriter, s set) {
	for method := range s {
		w.Header().Add("Access-Control-Allow-Methods", method)
	}
}

func setAllowCredentials(w http.ResponseWriter, b bool) {
	if b {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		return
	}
	w.Header().Set("Access-Control-Allow-Credentials", "false")
}

func headersAreAllowed(req *http.Request, s set) bool {
	for k := range req.Header {
		if !s.Contains(k, false) {
			return false
		}
	}
	return true
}

func methodIsAllowed(req *http.Request, s set) bool {
	return s.Contains(req.Method, true)
}

func contentTypeIsAllowed(req *http.Request, s set) bool {
	h := textproto.MIMEHeader(req.Header)
	ct := h.Get("Content-Type")
	return s.Contains(ct, false)

}

func originIsPresent(req *http.Request) bool {
	ori := textproto.MIMEHeader(req.Header).Get("Origin")
	if ori != "" {
		return true
	}
	return false
}

// set defines an unordered list of string elements.
// Two methods have been made available:
// - an insert method called `Add`
// - a delete method called `Remove`
// - a lookup method called `Contains`
type set map[string]struct{}

func newSet() set {
	s := make(map[string]struct{})
	return s
}

func (s set) Add(strls ...string) set {
	for _, str := range strls {
		s[str] = struct{}{}
	}
	return s
}

func (s set) Remove(str string, caseSensitive bool) {
	if !caseSensitive {
		str = strings.ToLower(str)
	}
	delete(s, str)
}

func (s set) Contains(str string, caseSensitive bool) bool {
	if !caseSensitive {
		str = strings.ToLower(str)
	}
	for k := range s {
		if k == str {
			return true
		}
	}
	return false
}
