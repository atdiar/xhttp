// Package cors implements the server-side logic that is employed in response
// to a Cross Origin request.
package cors

import (
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"github.com/atdiar/goroutine/execution"
	"github.com/atdiar/xhttp"
)

// Access control reference: https://www.w3.org/TR/cors/

/*
Rationale
=========

A Cross Origin http request for a given resource is made when a user agent is
used to retrieve resources from a  given domain which themselves depend on
resources from another domain (such as an image stored on a foreign  CDN for
instance).

The current package can be used to specify the conditions under which we allow a
resource at a given endpoint to be accessed.

The default being `same-origin` policy (same domain, same protocol, same port,
same host), it can be relaxed by specifying the type of Cross Origin request the
server allows (by Origin, by Headers, Content-type, etc.)

Hence, the presence of these headers determines whether a resource is accessible.
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

	// SimpleResponseHeaders is the set of header field names for which CORS is
	// allows a response to a request without preflight.
	SimpleResponseHeaders = newSet().Add("Cache-Control", "Content-Language", "Content-Type", "Expires", "Last-Modified", "Pragma")
)

// Handler is used to specify and enforce a Cross Origin Resource Sharing policy
// on incoming http requests.
// CORS controls the access to resources available on the server by defining
// constraints (request origin, http methods allowed, headers allowed, etc.)
type Handler struct {
	Parameters
	Preflight *PreflightHandler
	next      xhttp.Handler
}

// Parameters defines the set of actionable components that are used to define a
// response to a Cross-Origin request.
// "*" is used to denote that anything is accepted (resp. Headers, Methods,
// Content-Types).
// The fields AllowedOrigins, AllowedHeaders, AllowedMethods, ExposeHeaders and
// AllowedContentTypes are sets of strings. A string may be inserted by using
// the `Add(str string, caseSensitive bool)` method.
// It is also possible to lookup for the existence of a string within a set
// thanks to the `Contains(str string, caseSensitive bool)` method.
type Parameters struct {
	AllowedOrigins      set
	AllowedHeaders      set
	AllowedContentTypes set
	ExposeHeaders       set
	AllowedMethods      set
	AllowCredentials    bool
}

// PreflightHandler holds the elements required to build and register
// the http response logic to a preflight request.
type PreflightHandler struct {
	*Parameters
	MxAge time.Duration
	mux   *xhttp.ServeMux
	pat   string

	next xhttp.Handler
}

// MaxAge sets a limit to the validity of a preflight result in
// cache.
func (p *PreflightHandler) MaxAge(t time.Duration) {
	// Implementation which should set the Access-Control-Max-Age header in sec.
	// (in the allowed headers)
	p.AllowedHeaders.Add("Access-Control-Max-Age")
	p.MxAge = t

}

// NewHandler creates a new, CORS policy enforcing, request handler.
// By default, it enables Cross site simple requests without preflight.
func NewHandler() Handler {
	h := Handler{}
	h.Parameters.AllowedOrigins = newSet().Add("*")
	h.Parameters.AllowedHeaders = newSet().Add("Accept", "Accept-Language", "Content-Language", "Content-Type", "Origin")
	h.Parameters.AllowedContentTypes = newSet().Add("application/x-www-form-urlencoded", "multipart/form-data", "text/plain")
	return h
}

// EnablePreflight will allow the handling of preflighted requests via the
// OPTIONS http method.
// Preflight result mayt be cached by the client
func (h Handler) EnablePreflight(mux *xhttp.ServeMux, endpoint string) {
	h.Preflight = new(PreflightHandler)
	h.Preflight.Parameters = &h.Parameters
	h.Preflight.MxAge = 10 * time.Minute

	h.Preflight.Parameters.AllowedMethods = h.AllowedMethods.Add("OPTIONS")
	h.Preflight.AllowedHeaders.Add("Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers")

	mux.OPTIONS(endpoint, h.Preflight)
}

func (p *PreflightHandler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	// Check Headers: Origin, Access-Control-Request-Method, Access-Control-Request-Headers
	if !originIsPresent(r) {
		if p.next != nil {
			p.next.ServeHTTP(ctx, w, r)
		}
		return
	}

	// The preflight request is a preparation step that verifies that the request
	// obseves the requirement from the server in terms of origin, method, headers
	// 1. The server shall check that the origin is accepted (case sensitive match
	// in allowed headers).
	// If not, the request cannot be processed further.
	// 2. Check Access-Control-Request-Method. If absent, just return. The
	// response to the preflight will not have the necessary headers and the
	// user-agent will be able to determine that something went wrong.
	// 3.

	// Checking origin
	origin, ok := (textproto.MIMEHeader(r.Header))["Origin"]
	if !ok {
		if p.next != nil {
			p.next.ServeHTTP(ctx, w, r)
		}
		return
	}
	originallowed := p.Parameters.AllowedOrigins.Contains(origin[0], true)
	if p.Parameters.AllowedOrigins.Contains("*", false) {
		originallowed = true
	}
	if !originallowed {
		if p.next != nil {
			p.next.ServeHTTP(ctx, w, r)
		}
		return
	}

	// Checking method
	method, ok := (textproto.MIMEHeader(r.Header))["Access-Control-Request-Method"]
	if !ok {
		if p.next != nil {
			p.next.ServeHTTP(ctx, w, r)
		}
		return
	}
	methodallowed := p.Parameters.AllowedMethods.Contains(method[0], true)
	if p.Parameters.AllowedMethods.Contains("*", true) {
		methodallowed = true
	}
	if !methodallowed {
		if p.next != nil {
			p.next.ServeHTTP(ctx, w, r)
		}
		return
	}

	// Checking headers
	headers, ok := (textproto.MIMEHeader(r.Header))["Access-Control-Request-Headers"]
	if !ok {
		if p.next != nil {
			p.next.ServeHTTP(ctx, w, r)
		}
		return
	}
	headersallowed := p.Parameters.AllowedHeaders.Contains(headers[0], false)
	for _, header := range headers {
		headersallowed = headersallowed && p.Parameters.AllowedHeaders.Contains(header, false)
	}
	if p.Parameters.AllowedHeaders.Contains("*", false) {
		headersallowed = true
	}
	if !headersallowed {
		if p.next != nil {
			p.next.ServeHTTP(ctx, w, r)
		}
		return
	}

	// Setting the apporpriate Headers on the HTTP response
	setAllowCredentials(w, p.Parameters.AllowCredentials)

	if p.MxAge != 0 {
		setMaxAge(w, int(p.MxAge.Seconds()))
	}

	w.Header().Add("Access-Control-Allow-Methods", method[0])
	for _, header := range headers {
		w.Header().Add("Access-Control-Allow-Headers", header)
	}

	if p.next != nil {
		p.next.ServeHTTP(ctx, w, r)
	}
}

// Link enables the linking of a xhttp.Handler to the preflight request handler.
func (p *PreflightHandler) Link(h xhttp.Handler) xhttp.HandlerLinker {
	p.next = h
	return p
}

// WithCredentials will allow the emmission of cookies, authorization headers,
// TLS client certificates with the http requests by the client.
func (h Handler) WithCredentials() Handler {
	h.Parameters.AllowCredentials = true
	return h
}

func (h Handler) ServeHTTP(ctx execution.Context, w http.ResponseWriter, r *http.Request) {
	if !originIsPresent(r) {
		if h.next != nil {
			h.next.ServeHTTP(ctx, w, r)
		}
		return
	}

	// if the request is a simple one, we do not need to do much.
	if methodIsAllowed(r, SimpleRequestMethods) {
		if headersAreAllowed(r, SimpleRequestHeaders) {
			if contentTypeIsAllowed(r, SimpleRequestContentTypes) {
				if h.next != nil {
					h.next.ServeHTTP(ctx, w, r)
				}
				return
			}
		}
	}

	setAllowOrigin(w, r, h.AllowedOrigins)
	setAllowCredentials(w, h.AllowCredentials)
	setExposeHeaders(w, h.ExposeHeaders)

	if h.next != nil {
		h.next.ServeHTTP(ctx, w, r)
	}
}

// Link enables the linking of a xhttp.Handler to the cors request handler.
func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

// setAllowOrigin will write the Access-Control-Allow-Origin header assigning to
// it the correct value.
func setAllowOrigin(w http.ResponseWriter, r *http.Request, AllowedOrigins set) {
	ori := textproto.MIMEHeader(r.Header).Get("Origin")

	if !AllowedOrigins.Contains(ori, true) {
		if AllowedOrigins.Contains("*", true) {
			w.Header().Set("Access-Control-Allow-Origin", ori)
			return
		}

		w.Header().Set("Access-Control-Allow-Origin", "null")
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", ori)

}

// setAllowMethods will write the Access-Control-Allow-Methods header assigning to
// it the correct value. It is written in response to a preflight request to
// provide the user-agent with the list of methods that can be used in the actual
// request.
func setAllowMethods(w http.ResponseWriter, s set) {
	for method := range s {
		w.Header().Add("Access-Control-Allow-Methods", method)
	}
}

// setAllowHeaders will write the Access-Control-Allow-Headers header assigning to
// it the correct value. It is written in response to a preflight request to
// provide the user-agent with the list of headers that can be used in the actual
// request.
func setAllowHeaders(w http.ResponseWriter, s set) {
	for header := range s {
		w.Header().Add("Access-Control-Allow-Headers", header)
	}
}

// setExposeHeaders writes out the Access-Control-Expose-Headers header.
// This is merely a whitelist of headers that the user-agent can read from an
// http response to a CORS request.
func setExposeHeaders(w http.ResponseWriter, s set) {
	for header := range s {
		w.Header().Add("Access-Control-Expose-Headers", header)
	}
}

// setAllowCredentials writes out the Access-Control-Allow-Credentials header which
// indicates whether the the actual request can include user credentials (in the
// case of a preflighted request).
// Otherwise (no preflight), it indicates whether the response can be exposed.
//
// NOTE: Note sure it will be that useful since the Basic Authenitcation scheme
// of the http protocol is not very practical.
func setAllowCredentials(w http.ResponseWriter, b bool) {
	if b {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		return
	}
	w.Header().Set("Access-Control-Allow-Credentials", "false")
}

// setMaxAge writes out the Access-Control-Max-Age header which indicates for
// how long the results of the preflight request can be cached by the user-agent
// (browser for instance)
func setMaxAge(w http.ResponseWriter, seconds int) {
	w.Header().Set("Access-Control-Max-Age", strconv.Itoa(seconds))
}

func headersAreAllowed(r *http.Request, s set) bool {
	for k := range r.Header {
		if !s.Contains(k, false) {
			return false
		}
	}
	return true
}

func methodIsAllowed(r *http.Request, s set) bool {
	return s.Contains(r.Method, true)
}

func contentTypeIsAllowed(r *http.Request, s set) bool {
	h := textproto.MIMEHeader(r.Header)
	ct := h["Content-Type"]
	var res bool
	for _, val := range ct {
		res = res && s.Contains(val, false)
	}
	return res
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
