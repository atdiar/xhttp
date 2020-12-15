// Package csrf implements a request handler which generates a token to protect
// against Cross-Site Request Forgery.
package csrf

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"

	"context"

	"github.com/atdiar/xhttp"
	"github.com/atdiar/xhttp/handlers/session"
)

// TODO use subtle compare... it mitigates timing attacks apparently

const (
	methodGET     = "GET"
	methodHEAD    = "HEAD"
	methodOPTIONS = "OPTIONS"
	TokenInvalid  = "Forbidden. anti-CSRF Token/header missing or invalid"
	HeaderMissing = "Anti CSRF header is missing"
)

var (
	ErrInvalidSession = errors.New("Session does not exist ?")
)

// Handler is a special type of request handler that creates a token value used
// to protect against Cross-Site Request Forgery vulnerabilities.
type Handler struct {
	Header  string // Name of the anti-csrf request header to check
	Session session.Handler
	next    xhttp.Handler
}

// NewHandler builds a new anti-CSRF request handler, creating a full session
// object.
func NewHandler(name string, secret string, options ...func(Handler) Handler) Handler {
	h := Handler{}
	h.Session = session.New(name, secret)

	h.Header = "X-CSRF-TOKEN"

	h.Session.Cookie.HttpCookie.HttpOnly = false
	if options != nil {
		for _, opt := range options {
			h = opt(h)
		}
	}
	return h
}

// Link enables the linking of a xhttp.Handler to the anti-CSRF request Handler.
func (h Handler) Link(hn xhttp.Handler) xhttp.HandlerLinker {
	h.next = hn
	return h
}

func (h Handler) generateToken(ctx context.Context, res http.ResponseWriter, req *http.Request) (context.Context, error) {
	tok, err := generateToken(32)
	if err != nil {
		http.Error(res, "Generating anti-CSRF Token failed", 503)
		return ctx, err
	}
	// First we replace the session cookie by the anti-CSRF cookie
	// That will ensure that on Session Save, the anti-CSRF is registered in the
	// http response header.
	err = h.Session.Put(ctx, h.Session.Name, []byte(tok), 0)

	if err != nil {
		http.Error(res, "Storing new CSRF Token in session failed", 503)
		return ctx, err
	}
	return h.Session.Save(ctx, res, req)
}

// CtxToken returns the encoded session value of a csrf token.
func (h Handler) CtxToken(ctx context.Context) (string, error) {
	c, ok := ctx.Value(h.Session.ContextKey).(http.Cookie)
	if !ok {
		return "", errors.New("CSRF: could not retrieve anticsrf token. Absent")
	}
	return c.Value, nil
}

// ServeHTTP handles the servicing of incoming http requests.
func (h Handler) ServeHTTP(ctx context.Context, res http.ResponseWriter, req *http.Request) {
	// We want any potential caching system to remain aware of changes to the
	// cookie header. As such, we have to add a Vary header.
	res.Header().Add("Vary", "Cookie")

	// First we have to load the session data.
	// Indeed, we want to register the CSRF token as a session value.
	// For this, we need to use the most recently generated session id.
	ctx, err := h.Session.Load(ctx, res, req)

	switch req.Method {
	case methodGET, methodHEAD, methodOPTIONS:
		// No CSRF check is performed for these methods.
		// However, an anti-CSRF token is generated and sent with the response
		// iff none has been generated yet.
		if err != nil {
			ctx, err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
		}
		if h.next != nil {
			h.next.ServeHTTP(ctx, res, req)
		}

	default:
		if err != nil {
			ctx, err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
			http.Error(res, TokenInvalid, 403)
			return
		}

		Header, ok := req.Header[h.Header]
		if !ok {
			http.Error(res, HeaderMissing, http.StatusBadRequest)
			return
		}

		// Validation

		// Header exists. The anti-csrf cookie must be present too.
		headerToken := Header[0]
		cookie, ok := ctx.Value(h.Session.ContextKey).(http.Cookie)
		if !ok {
			ctx, err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
			http.Error(res, "anti csrf session not valid", http.StatusBadRequest)
			return
		}
		cookieToken := cookie.Value
		if headerToken != cookieToken {
			ctx, err = h.generateToken(ctx, res, req)
			if err != nil {
				http.Error(res, "Internal Server Error", 500)
				return
			}
			http.Error(res, "anti csrf header not valid", http.StatusBadRequest)
			return
		}
		if h.next != nil {
			h.next.ServeHTTP(ctx, res, req)
		}
		return
	}
}

// generateToken creates a base64 encoded version of a 32byte Cryptographically
// secure random number to be used as a protection against CSRF attacks.
// It uses Go's implementation of devurandom (which has a backup in case
// devurandom is inaccessible)
func generateToken(length int) (string, error) {
	bstr := make([]byte, length)
	_, err := rand.Read(bstr)
	if err != nil {
		return "", err
	}
	str := base64.StdEncoding.EncodeToString(bstr)
	return str, nil
}
